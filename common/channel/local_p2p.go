package channel

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	upnp "github.com/NebulousLabs/go-upnp"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/common/protector"
	"github.com/yinqiwen/pmux"
)

const (
	unknownRole   = 0
	p2pClientRole = 1
	p2pServerRole = 2
)

var UPNPExposePort int

type p2pHandshakeCtx struct {
	opt *p2pHandshakeOptions
	ch  *LocalProxyChannel
}

var upnpIGD *upnp.IGD
var upnpIGDInited bool
var upnpMappingPort = int(-1)
var upnpExternalIP string
var upnpIGDLock sync.Mutex

var p2pHandshakeCtxTable sync.Map

type p2pHandshakeOptions struct {
	role int
}

func handleP2PConn(p2pConn net.Conn, ch *LocalProxyChannel, opt *p2pHandshakeOptions) error {
	logger.Info("Accept P2P conn from %v to %v", p2pConn.RemoteAddr(), p2pConn.LocalAddr())
	if nil == ch {
		rhost, _, _ := net.SplitHostPort(p2pConn.RemoteAddr().String())
		st := time.Now()
		for {
			v, exist := p2pHandshakeCtxTable.Load(rhost)
			if !exist {
				if time.Now().Sub(st) >= 10*time.Second {
					break
				}
				time.Sleep(50 * time.Millisecond)
				continue
			}
			ch = v.(*p2pHandshakeCtx).ch
			opt = v.(*p2pHandshakeCtx).opt
			break
		}
		if nil == ch {
			p2pConn.Close()
			logger.Error("Failed to get p2p handshake context by:%v", rhost)
			return fmt.Errorf("No p2p handshake context found for %s", rhost)
		}
	}
	for opt.role == unknownRole {
		time.Sleep(50 * time.Millisecond)
	}
	var p2pSession mux.MuxSession
	var recvAuth *mux.AuthRequest
	var err error
	if p2pClientRole == opt.role {
		err, recvAuth, _, p2pSession = clientAuthConn(p2pConn, ch.Conf.Cipher.Method, &ch.Conf, true)
		if nil != err {
			logger.Error("Failed to auth to remote p2p node:%v with err:%v", p2pConn.RemoteAddr(), err)
			p2pConn.Close()
			return err
		}
	} else {
		session, err := pmux.Server(p2pConn, InitialPMuxConfig(&DefaultServerCipher))
		if nil != err {
			logger.Error("[ERROR]Failed to create mux session for tcp server with reason:%v", err)
			p2pConn.Close()
			return err
		}
		p2pSession = &mux.ProxyMuxSession{Session: session}
		recvAuth, err = serverAuthSession(p2pSession, nil, true)
		if nil != err {
			p2pConn.Close()
			return err
		}
	}
	ch.setP2PSession(p2pConn, p2pSession, recvAuth)
	return nil
}

func servTCP(lp net.Listener, ch *LocalProxyChannel, opt *p2pHandshakeOptions) {
	for {
		p2pConn, err := lp.Accept()
		if nil != err {
			return
		}
		handleP2PConn(p2pConn, ch, opt)
	}
}

func startP2PServer(addr string, ch *LocalProxyChannel, hopt *p2pHandshakeOptions) (net.Listener, error) {
	laddr, err := net.ResolveTCPAddr("tcp", addr)
	opt := &protector.NetOptions{
		ReusePort: protector.SupportReusePort(),
	}
	lp, err := protector.ListenTCP(laddr, opt)
	if nil != err {
		logger.Error("[ERROR]Failed to listen P2P TCP address:%s with reason:%v", addr, err)
		return nil, err
	}
	logger.Info("Listen on P2P TCP address:%s", addr)
	go servTCP(lp, ch, hopt)
	return lp, nil
}

func initUDNP() error {
	if upnpIGDInited {
		return nil
	}
	upnpIGDInited = true
	logger.Info("Start get upnp")
	d, err := upnp.DiscoverCtx(context.Background())
	if err != nil {
		logger.Error("Failed to discover upnp device with error:%v", err)
		return err
	}
	logger.Info("Success init upnp.")
	upnpIGD = d
	return nil
}

func startUPNPServer() error {
	upnpIGDLock.Lock()
	defer upnpIGDLock.Unlock()
	err := initUDNP()
	if nil == upnpIGD {
		return err
	}
	if len(upnpExternalIP) > 0 {
		return nil
	}
	extIP, err := upnpIGD.ExternalIP()
	if nil != err {
		logger.Error("Failed to get external IP with err:%v", err)
		return err
	}
	lp, err := startP2PServer(fmt.Sprintf("0.0.0.0:%d", UPNPExposePort), nil, nil)
	if nil != err {
		logger.Error("Failed to listen on %s with err:%v", err)
		return err
	}
	upnpMappingPort = lp.(*net.TCPListener).Addr().(*net.TCPAddr).Port

	err = upnpIGD.Forward(uint16(upnpMappingPort), "gsnova_upnp")
	if nil != err {
		logger.Error("Failed to add port mapping with upnp by error:%v", err)
		lp.Close()
		upnpMappingPort = -1
		return err
	}
	logger.Info("UPNP IP:%s with port:%d success .", extIP, upnpMappingPort)
	upnpExternalIP = extIP
	return nil
}

func startP2PSession(server string, pch LocalChannel, ch *LocalProxyChannel) error {
	logger.Info("Start P2P TCP client to %s ", server)
	maxRetry := 10
	waitAfterErr := 3 * time.Second
	var priLocalAddr, pubLocalAddr string
	var lastFailPeerPriIP, lastFailPubPeerIP string
	var p2pLis net.Listener
	p2pOpt := &p2pHandshakeOptions{}
	if UPNPExposePort > 0 {
		startUPNPServer()
	}
	opt := &protector.NetOptions{
		ReusePort:   protector.SupportReusePort(),
		DialTimeout: 3 * time.Second,
		//LocalAddr: priLocalAddr,
	}
	handshakeCtx := &p2pHandshakeCtx{
		opt: p2pOpt,
		ch:  ch,
	}
	for {
		if ch.isP2PSessionEstablisehd() {
			time.Sleep(3 * time.Second)
			continue
		}
		session, err := pch.CreateMuxSession(server, &ch.Conf)
		if nil != err || nil == session {
			logger.Error("Failed to init mux session:%v", err)
			if nil != session {
				session.Close()
			}
			time.Sleep(waitAfterErr)
			continue
		}
		//ps := &mux.ProxyMuxSession{Session: session}
		priLocalAddr = session.LocalAddr().String()
		if upnpMappingPort > 0 {
			host, _, _ := net.SplitHostPort(priLocalAddr)
			priLocalAddr = fmt.Sprintf("%s:%d", host, upnpMappingPort)
			pubLocalAddr = fmt.Sprintf("%s:%d", upnpExternalIP, upnpMappingPort)
		} else {
			pubLocalAddr = ""
		}
		var peerPriAddr, peerPubAddr string
		cipherMethod := ch.Conf.Cipher.Method
		isFirst := true
		for len(peerPriAddr) == 0 {
			//logger.Debug("Peer address is empty.")
			err, authReq, authRes := clientAuthMuxSession(session, cipherMethod, &ch.Conf, priLocalAddr, pubLocalAddr, isFirst, false)
			isFirst = false
			if nil != err {
				logger.Error("Failed to auth p2p with err:%v", err)
				time.Sleep(1 * time.Second)
				session.Close()
				break
			}
			priLocalAddr = authReq.P2PPriAddr
			peerPriAddr = authRes.PeerPriAddr
			peerPubAddr = authRes.PeerPubAddr
			if len(authRes.PubAddr) > 0 {
				pubLocalAddr = authRes.PubAddr
			}

			if len(peerPubAddr) == 0 {
				time.Sleep(waitAfterErr)
				continue
			}
			if len(lastFailPeerPriIP) > 0 {
				//do not try to connect remote with same failed IPs
				if strings.HasPrefix(peerPriAddr, lastFailPeerPriIP+":") && strings.HasPrefix(peerPubAddr, lastFailPubPeerIP+":") {
					peerPriAddr = ""
					peerPubAddr = ""
					time.Sleep(waitAfterErr)
					continue
				}
			}
		}
		if len(peerPubAddr) == 0 {
			time.Sleep(waitAfterErr)
			session.Close()
			continue
		}

		if strings.Compare(pubLocalAddr, peerPubAddr) < 0 {
			p2pOpt.role = p2pClientRole
		} else {
			p2pOpt.role = p2pServerRole
		}
		logger.Info("Recv P2P peer private address:%s & public address:%s", peerPriAddr, peerPubAddr)

		{
			rhost, _, _ := net.SplitHostPort(peerPubAddr)
			p2pHandshakeCtxTable.Store(rhost, handshakeCtx)
			rhost, _, _ = net.SplitHostPort(peerPriAddr)
			p2pHandshakeCtxTable.Store(rhost, handshakeCtx)
		}

		tryStartTime := time.Now()
		lisAddr := priLocalAddr
		if opt.ReusePort && upnpMappingPort <= 0 {
			p2pLis, err = startP2PServer(lisAddr, ch, p2pOpt)
			if nil != err {
				logger.Error("Failed to listen on %s with err:%v", lisAddr, err)
				time.Sleep(waitAfterErr)
				session.Close()
				continue
			}
		}

		peerConnOpt := &protector.NetOptions{
			ReusePort:   opt.ReusePort,
			LocalAddr:   priLocalAddr,
			DialTimeout: 3 * time.Second,
		}
		if !opt.ReusePort || upnpMappingPort > 0 {
			peerConnOpt.LocalAddr = ""
			peerConnOpt.ReusePort = false
		}

		sig := make(chan int)

		connFunc := func(addr string) {
			for i := 0; i < maxRetry && !ch.isP2PSessionEstablisehd(); i++ {
				var p2pConn net.Conn
				var cerr error
				if len(ch.Conf.Proxy) > 0 {
					p2pConn, cerr = helper.ProxyDial(ch.Conf.Proxy, opt.LocalAddr, addr, opt.DialTimeout, true)
				} else {
					p2pConn, cerr = protector.DialContextOptions(context.Background(), "tcp", addr, peerConnOpt)
				}

				if nil != cerr {
					logger.Error("Failed to connect peer %s with err:%v", addr, cerr)
					time.Sleep(1 * time.Second)
					continue
				}

				logger.Info("P2P connection established %s->%s", p2pConn.LocalAddr(), p2pConn.RemoteAddr())
				if nil != handleP2PConn(p2pConn, ch, p2pOpt) {
					time.Sleep(waitAfterErr)
					continue
				}
				break
			}
			sig <- 1
		}

		go connFunc(peerPriAddr)
		go connFunc(peerPubAddr)

		<-sig
		<-sig
		if nil != p2pLis {
			p2pLis.Close()
			p2pLis = nil
		}
		p2pOpt.role = unknownRole
		session.Close()
		for time.Now().Sub(tryStartTime) < time.Duration(maxRetry)*waitAfterErr {
			time.Sleep(1 * time.Second)
		}
		if !ch.isP2PSessionEstablisehd() {
			lastFailPeerPriIP, _, _ = net.SplitHostPort(peerPriAddr)
			lastFailPubPeerIP, _, _ = net.SplitHostPort(peerPubAddr)
		}
	}
}

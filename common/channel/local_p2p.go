package channel

import (
	"context"
	"net"
	"strings"
	"time"

	upnp "github.com/NebulousLabs/go-upnp"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/common/protector"
	"github.com/yinqiwen/pmux"
)

func servTCP(lp net.Listener, ch *LocalProxyChannel) {
	for {
		conn, err := lp.Accept()
		if nil != err {
			return
		}
		logger.Info("Accept P2P conn from %v to %v", conn.RemoteAddr(), conn.LocalAddr())
		session, err := pmux.Server(conn, InitialPMuxConfig(&DefaultServerCipher))
		if nil != err {
			logger.Error("[ERROR]Failed to create mux session for tcp server with reason:%v", err)
			continue
		}
		muxSession := &mux.ProxyMuxSession{Session: session}
		recvAuth, err := serverAuthSession(muxSession, nil, true)
		if nil != err {
			continue
		}
		ch.setP2PSession(conn, muxSession, recvAuth)
	}
}

func startP2PServer(addr string, ch *LocalProxyChannel) (net.Listener, error) {
	laddr, _ := net.ResolveTCPAddr("tcp", addr)
	opt := &protector.NetOptions{
		ReusePort: true,
	}
	lp, err := protector.ListenTCP(laddr, opt)
	if nil != err {
		logger.Error("[ERROR]Failed to listen P2P TCP address:%s with reason:%v", addr, err)
		return nil, err
	}
	logger.Info("Listen on P2P TCP address:%s", addr)
	go servTCP(lp, ch)
	return lp, nil
}

func initUDNP() (*upnp.IGD, error) {
	d, err := upnp.DiscoverCtx(context.Background())
	if err != nil {
		logger.Error("Failed to discover upnp device with error:%v", err)
		return nil, err
	}
	return d, nil
}

func startP2PSession(server string, pch LocalChannel, ch *LocalProxyChannel) error {
	logger.Info("Start P2P TCP client to %s", server)
	maxRetry := 10
	waitAfterErr := 3 * time.Second
	var priLocalAddr, pubLocalAddr string
	var lastFailPeerPriIP, lastFailPubPeerIP string
	var upnpIGD *upnp.IGD
	if ch.Conf.WithUPNP {
		upnpIGD, _ = initUDNP()
	}
	for {
		if ch.isP2PSessionEstablisehd() {
			time.Sleep(3 * time.Second)
			continue
		}
		opt := &protector.NetOptions{
			ReusePort:   true,
			DialTimeout: 3 * time.Second,
			//LocalAddr: priLocalAddr,
		}
		session, err := pch.CreateMuxSession(server, &ch.Conf)
		if nil != err {
			logger.Error("Failed to init mux session:%v", err)
			session.Close()
			time.Sleep(waitAfterErr)
			continue
		}
		//ps := &mux.ProxyMuxSession{Session: session}
		priLocalAddr = session.LocalAddr().String()

		var peerPriAddr, peerPubAddr string
		cipherMethod := ch.Conf.Cipher.Method
		isFirst := true
		for len(peerPriAddr) == 0 {
			//logger.Debug("Peer address is empty.")
			err, authReq, authRes := clientAuthMuxSession(session, cipherMethod, &ch.Conf, priLocalAddr, isFirst, false)
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
			pubLocalAddr = authRes.PubAddr
			if len(peerPriAddr) == 0 {
				time.Sleep(1 * time.Second)
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
		if len(peerPriAddr) == 0 {
			time.Sleep(waitAfterErr)
			session.Close()
			continue
		}
		logger.Info("Recv P2P peer private address:%s & public address:%s", peerPriAddr, peerPubAddr)

		tryStartTime := time.Now()
		lp, err := startP2PServer(priLocalAddr, ch)
		if nil != err {
			logger.Error("Failed to listen on %s with err:%v", priLocalAddr, err)
			time.Sleep(waitAfterErr)
			session.Close()
			continue
		}
		upnpMappingPort := -1
		if nil != upnpIGD {
			upnpMappingPort = session.LocalAddr().(*net.TCPAddr).Port
			err = upnpIGD.Forward(uint16(upnpMappingPort), "gsnova_"+ch.Conf.P2PToken)
			if nil != err {
				logger.Error("Failed to add port mapping with upnp by error:%v", err)
				upnpMappingPort = -1
			}
		}
		peerConnOpt := &protector.NetOptions{
			ReusePort:   true,
			LocalAddr:   priLocalAddr,
			DialTimeout: 3 * time.Second,
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
				asClient := false
				if strings.Compare(pubLocalAddr, peerPubAddr) < 0 {
					asClient = true
				}
				var p2pSession mux.MuxSession
				var recvAuth *mux.AuthRequest
				if asClient {
					err, recvAuth, _, p2pSession = clientAuthConn(p2pConn, cipherMethod, &ch.Conf, true)
					if nil != err {
						logger.Error("Failed to auth to remote p2p node:%v with err:%v", p2pConn.RemoteAddr(), err)
						time.Sleep(waitAfterErr)
						p2pConn.Close()
						continue
					}
				} else {
					session, err := pmux.Server(p2pConn, InitialPMuxConfig(&DefaultServerCipher))
					if nil != err {
						logger.Error("[ERROR]Failed to create mux session for tcp server with reason:%v", err)
						time.Sleep(waitAfterErr)
						p2pConn.Close()
						continue
					}
					p2pSession = &mux.ProxyMuxSession{Session: session}
					recvAuth, err = serverAuthSession(p2pSession, nil, true)
					if nil != err {
						time.Sleep(waitAfterErr)
						p2pConn.Close()
						continue
					}
				}
				ch.setP2PSession(p2pConn, p2pSession, recvAuth)
				break
			}
			sig <- 1
		}

		go connFunc(peerPriAddr)
		go connFunc(peerPubAddr)

		<-sig
		<-sig
		lp.Close()

		if upnpMappingPort > 0 {
			err = upnpIGD.Clear(uint16(upnpMappingPort))
			if nil != err {
				logger.Error("Failed to clear port mapping with upnp by error:%v", err)
			}
		}
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

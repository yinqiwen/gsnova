package channel

import (
	"context"
	"net"
	"strings"
	"time"

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

func startP2PSession(raddr string, ch *LocalProxyChannel) error {
	logger.Info("Start P2P TCP client to %s", raddr)
	maxRetry := 10
	waitAfterErr := 3 * time.Second
	var priLocalAddr, pubLocalAddr string
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
		c, err := protector.DialContextOptions(context.Background(), "tcp", raddr, opt)
		if nil != err {
			logger.Error("Failed to connect %s with err:%v", raddr, err)
			time.Sleep(waitAfterErr)
			continue
		}
		session, err := pmux.Client(c, InitialPMuxConfig(&ch.Conf.Cipher))
		if nil != err {
			logger.Error("Failed to init mux session:%v", err)
			c.Close()
			time.Sleep(waitAfterErr)
			continue
		}
		ps := &mux.ProxyMuxSession{Session: session}
		priLocalAddr = c.LocalAddr().String()

		var peerPriAddr, peerPubAddr string
		cipherMethod := ch.Conf.Cipher.Method
		isFirst := true
		for len(peerPriAddr) == 0 {
			//logger.Debug("Peer address is empty.")
			err, authReq, authRes := clientAuthMuxSession(ps, cipherMethod, &ch.Conf, priLocalAddr, isFirst, false)
			isFirst = false
			if nil != err {
				logger.Error("Failed to auth p2p with err:%v", err)
				time.Sleep(1 * time.Second)
				c.Close()
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
		}
		if len(peerPriAddr) == 0 {
			time.Sleep(waitAfterErr)
			c.Close()
			continue
		}
		logger.Info("Recv P2P peer private address:%s & public address:%s", peerPriAddr, peerPubAddr)

		lp, err := startP2PServer(priLocalAddr, ch)
		if nil != err {
			logger.Error("Failed to listen on %s with err:%v", priLocalAddr, err)
			time.Sleep(waitAfterErr)
			c.Close()
			continue
		}
		peerConnOpt := &protector.NetOptions{
			ReusePort:   true,
			LocalAddr:   priLocalAddr,
			DialTimeout: 3 * time.Second,
		}
		sig := make(chan int)

		connFunc := func(addr string) {
			for i := 0; i < maxRetry && !ch.isP2PSessionEstablisehd(); i++ {
				p2pConn, cerr := protector.DialContextOptions(context.Background(), "tcp", addr, peerConnOpt)
				if nil != cerr {
					logger.Error("Failed to connect peer %s with err:%v", addr, cerr)
					time.Sleep(1 * time.Second)
					continue
				}

				logger.Info("P2P connection established %s->%s", c.LocalAddr(), c.RemoteAddr())
				asClient := false
				if strings.Compare(pubLocalAddr, peerPubAddr) < 0 {
					asClient = true
				}
				var p2pSession mux.MuxSession
				var recvAuth *mux.AuthRequest
				if asClient {
					err, recvAuth, _, p2pSession = clientAuthConn(p2pConn, cipherMethod, &ch.Conf, true)
					if nil != err {
						logger.Error("Failed to auth to remote p2p node:%v with err:%v", c.RemoteAddr(), err)
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
		c.Close()
	}
}

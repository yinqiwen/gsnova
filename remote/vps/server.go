package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"

	quic "github.com/lucas-clemente/quic-go"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/remote"
)

var totalConn int32

type vpsServer struct {
	port       uint32
	aliveConns int32
	createTime time.Time
	retireTime time.Time
	lp         net.Listener
	quicLP     quic.Listener
}

var activeDynamicServers = make(map[*vpsServer]bool)
var retiredDynamicServers = make(map[*vpsServer]bool)
var dynamicServerMutex sync.Mutex

func activeDynamicServerSize() int {
	dynamicServerMutex.Lock()
	defer dynamicServerMutex.Unlock()
	return len(activeDynamicServers)
}
func retiredDynamicServerSize() int {
	dynamicServerMutex.Lock()
	defer dynamicServerMutex.Unlock()
	return len(retiredDynamicServers)
}

func selectDynamicVPServer() *vpsServer {
	var vs *vpsServer
	if len(remote.ServerConf.CandidateDynamicPort) > 0 && remote.ServerConf.MaxDynamicPort > 0 {
		remote.ServerConf.MaxDynamicPort = len(remote.ServerConf.CandidateDynamicPort)
	}
	dynamicServerMutex.Lock()
	if len(activeDynamicServers) >= remote.ServerConf.MaxDynamicPort {
		now := time.Now()
		lifeCycle := remote.ServerConf.DynamicPortLifeCycle
		if lifeCycle == 0 {
			lifeCycle = 1800
		}
		for vss := range activeDynamicServers {
			if len(remote.ServerConf.CandidateDynamicPort) == 0 && vss.createTime.Add(time.Duration(lifeCycle)*time.Second).Before(now) {
				vss.retireTime = now
				retiredDynamicServers[vss] = true
				delete(activeDynamicServers, vss)
			} else {
				if nil == vs || vss.aliveConns < vs.aliveConns {
					vs = vss
				}
			}
		}
	}
	if nil == vs {
		vs = new(vpsServer)
		var err error
		dynAddr := ":0"
		if len(remote.ServerConf.CandidateDynamicPort) > 0 {
			for _, port := range remote.ServerConf.CandidateDynamicPort {
				dynAddr = fmt.Sprintf(":%d", port)
				err = startDynamicServer(dynAddr, vs)
				if nil == err {
					break
				}
			}
		} else {
			err = startDynamicServer(dynAddr, vs)

		}
		if nil == err {
			activeDynamicServers[vs] = true
		} else {
			vs = nil
		}
	}
	clearExpiredDynamicVPServer()
	dynamicServerMutex.Unlock()
	return vs
}

func clearExpiredDynamicVPServer() {
	now := time.Now()
	for vs := range retiredDynamicServers {
		if vs.aliveConns == 0 && vs.retireTime.Add(10*time.Second).Before(now) {
			log.Printf("Close dynamic listen server :%d", vs.port)
			vs.lp.Close()
			delete(retiredDynamicServers, vs)
		}
	}
}

func serveProxyConn(conn helper.ProxyChannelConnection, vs *vpsServer) {
	if nil != vs {
		atomic.AddInt32(&vs.aliveConns, 1)
	}
	atomic.AddInt32(&totalConn, 1)
	bufconn := bufio.NewReader(conn)

	deferFunc := func() {
		conn.Close()
		atomic.AddInt32(&totalConn, -1)
		if nil != vs {
			atomic.AddInt32(&vs.aliveConns, -1)
		}
	}
	defer deferFunc()

	ctx := remote.NewConnContext()
	writeEvents := func(evs []event.Event, buf *bytes.Buffer) error {
		if len(evs) > 0 {
			buf.Reset()
			for _, ev := range evs {
				if nil != ev {
					event.EncryptEvent(buf, ev, &ctx.CryptoContext)
				}
			}
			if buf.Len() > 0 {
				conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
				b := buf.Bytes()
				_, err := conn.Write(b)

				return err
			}
		}
		return nil
	}

	var rbuf bytes.Buffer
	var wbuf bytes.Buffer

	writeTaskRunning := false
	connClosed := false
	reader := &helper.BufferChunkReader{bufconn, nil}
	for !connClosed {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		rbuf.Grow(8192)
		rbuf.ReadFrom(reader)
		if nil != reader.Err {
			conn.Close()
			connClosed = true
		}
		ress, err := remote.HandleRequestBuffer(&rbuf, ctx)
		if nil != err {
			if err != event.EBNR {
				log.Printf("[ERROR]connection %s:%d error:%v", ctx.User, ctx.ConnIndex, err)
				conn.Close()
				connClosed = true
				return
			}
		} else {
			writeEvents(ress, &wbuf)
			if !writeTaskRunning && len(ctx.User) > 0 && ctx.ConnIndex >= 0 {
				writeTaskRunning = true
				go func() {
					var lastEventTime time.Time
					queue := remote.GetEventQueue(ctx.ConnId, true)
					for !connClosed {
						evs, err := queue.PeekMulti(10, 1*time.Millisecond, false)
						now := time.Now()
						if ctx.Closing {
							evs = []event.Event{&event.ChannelCloseACKEvent{}}
							if remote.ServerConf.MaxDynamicPort > 0 {
								nvs := selectDynamicVPServer()
								if nil != nvs {
									evs = append(evs, &event.PortUnicastEvent{Port: nvs.port})
								}
							}
						} else {
							if nil != err {
								if remote.GetSessionTableSize() > 0 && lastEventTime.Add(10*time.Second).Before(now) {
									evs = []event.Event{event.NewHeartBeatEvent()}
								} else {
									continue
								}
							}
						}

						err = writeEvents(evs, &wbuf)
						if nil != err {
							log.Printf("TCP write error####:%v %d", err, len(evs))
						} else {
							queue.DiscardPeeks(false)
						}
						if ctx.Closing {
							break
						}
						lastEventTime = now
						if nil != err {
							log.Printf("TCP write error:%v", err)
							conn.Close()
							break
						} else {
							//queue.DiscardPeeks(false)
						}
					}
					remote.ReleaseEventQueue(queue)
				}()
			}
		}
	}
}
func listenTCPServer(addr string, vs *vpsServer) (net.Listener, error) {
	if len(addr) == 0 {
		log.Fatalf("Empty listen address.")
		return nil, nil
	}

	//var lp *net.TCPListener
	lp, err := net.Listen("tcp", addr)
	if nil != err {
		log.Printf("Can NOT listen on address:%s with reason:%v", addr, err)
		return nil, err
	}
	if len(remote.ServerConf.TLS.Cert) > 0 {
		tlscfg := &tls.Config{}
		tlscfg.Certificates = make([]tls.Certificate, 1)
		tlscfg.Certificates[0], err = tls.LoadX509KeyPair(remote.ServerConf.TLS.Cert, remote.ServerConf.TLS.Key)
		if nil != err {
			log.Fatalf("Invalid cert/key for reason:%v", err)
			return nil, nil
		}
		lp = tls.NewListener(lp, tlscfg)
	}
	tcpaddr := lp.Addr().(*net.TCPAddr)
	log.Printf("Listen on address %v", tcpaddr)
	if nil != vs {
		vs.port = uint32(tcpaddr.Port)
		vs.lp = lp
		vs.createTime = time.Now()
	}
	return lp, nil
}

func listenQUICServer(addr string, vs *vpsServer) (quic.Listener, error) {
	if len(addr) == 0 {
		log.Fatalf("Empty listen address.")
		return nil, nil
	}

	lp, err := quic.ListenAddr(addr, generateTLSConfig(), nil)
	if nil != err {
		return nil, err
	}

	udpaddr := lp.Addr().(*net.UDPAddr)
	log.Printf("Listen on address %v", udpaddr)
	if nil != vs {
		vs.port = uint32(udpaddr.Port)
		vs.quicLP = lp
		vs.createTime = time.Now()
	}
	return lp, nil
}

func startDynamicServer(addr string, vs *vpsServer) error {
	lp, err := listenTCPServer(addr, vs)
	if nil != err {
		return err
	}
	quicLP, qerr := listenQUICServer(addr, vs)
	if nil != qerr {
		return qerr
	}
	go func() {
		for {
			conn, err := lp.Accept()
			if nil != err {
				log.Printf("Accept %s error:%v", addr, err)
				return
			}
			go serveProxyConn(conn, vs)
		}
	}()
	go func() {
		for {
			sess, err := quicLP.Accept()
			if nil != err {
				continue
			}
			go func() {
				stream, err := sess.AcceptStream()
				if err != nil {
					log.Printf("Accept %s error:%v", addr, err)
					return
				}
				go serveProxyConn(stream, vs)
			}()

		}
	}()
	return nil
}

func startLocalProxyServer(addr string) error {
	lp, err := listenTCPServer(addr, nil)
	if nil != err {
		return err
	}
	for {
		conn, err := lp.Accept()
		if nil != err {
			continue
		}
		go serveProxyConn(conn, nil)
	}
	return nil
}

func startLocalQUICProxyServer(addr string) error {
	lp, err := quic.ListenAddr(addr, generateTLSConfig(), nil)
	if nil != err {
		return err
	}
	for {
		sess, err := lp.Accept()
		if nil != err {
			continue
		}
		stream, err := sess.AcceptStream()
		if err != nil {
			continue
		}
		go serveProxyConn(stream, nil)
	}
}

// Setup a bare-bones TLS config for the server
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}
}

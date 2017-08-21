package proxy

import (
	"bufio"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/local/socks"
)

var sidSeed uint32 = 0
var proxyServerRunning = true

func getSessionId() uint32 {
	return atomic.AddUint32(&sidSeed, 1)
}

func serveProxyConn(conn net.Conn, proxy ProxyConfig) {
	var p Proxy
	protocol := "tcp"
	sid := getSessionId()
	session := newProxySession(sid, conn)
	defer closeProxySession(sid)

	remoteHost := ""
	remotePort := ""
	//indicate that if remote opened by event
	tryResolveHost := false
	isSocksProxy := false
	isHttpsProxy := false
	isHttp11Proto := false
	var initialHTTPReq *http.Request
	socksConn, bufconn, err := socks.NewSocksConn(conn)

	if nil == err {
		isSocksProxy = true
		log.Printf("Local proxy recv %s proxy conn to %s", socksConn.Version(), socksConn.Req.Target)
		socksConn.Grant(&net.TCPAddr{
			IP: net.ParseIP("0.0.0.0"), Port: 0})
		session.LocalConn = socksConn
		if socksConn.Req.Target == GConf.UDPGWAddr {
			log.Printf("Handle udpgw conn for %v", socksConn.Req.Target)
			handleUDPGatewayConn(session, proxy)
			return
		}
		conn = socksConn

		remoteHost, remotePort, err = net.SplitHostPort(socksConn.Req.Target)
		if nil != err {
			log.Printf("Invalid socks target addresss:%s with reason %v", socksConn.Req.Target, err)
			return
		}
		if net.ParseIP(remoteHost) != nil && !helper.IsPrivateIP(remoteHost) && proxy.SNISniff {
			//this is a ip from local dns query
			tryResolveHost = true
		}
	} else {
		if nil == bufconn {
			conn.Close()
			return
		}
	}

	if nil == bufconn {
		bufconn = bufio.NewReader(conn)
	}
	defer conn.Close()

	//1. sniff SNI first
	if tryResolveHost {
		sniChunkPeekSize := 1024
		for {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			sniChunk, _ := bufconn.Peek(sniChunkPeekSize)
			if len(sniChunk) > 0 {
				sni, err := helper.TLSParseSNI(sniChunk)
				if err != nil {
					if err != helper.ErrTLSIncomplete {
						break
					}
					sniChunkPeekSize = sniChunkPeekSize * 2
					continue
				} else {
					log.Printf("Sniffed SNI:%s:%s for IP:%s:%s", sni, remotePort, remoteHost, remotePort)
					remoteHost = sni
					//p = proxy.getProxyByHostPort(protocol, net.JoinHostPort(remoteHost, remotePort))
					tryResolveHost = false
				}
			} else {
				//try next round
				break
			}
		}
	}

	//2. try read http
	if tryResolveHost {
		headChunk, _ := bufconn.Peek(7)
		if len(headChunk) != 7 {
			goto START
		}
		method := string(headChunk)
		if tmp := strings.Fields(method); len(tmp) > 0 {
			method = tmp[0]
		}
		method = strings.ToUpper(method)
		switch method {
		case "GET":
			fallthrough
		case "POST":
			fallthrough
		case "HEAD":
			fallthrough
		case "PUT":
			fallthrough
		case "DELETE":
			fallthrough
		case "CONNECT":
			fallthrough
		case "OPTIONS":
			fallthrough
		case "TRACE":
			fallthrough
		case "PATCH":
			isHttp11Proto = true
		default:
			isHttp11Proto = false
		}

		if isHttp11Proto {
			initialHTTPReq, err = http.ReadRequest(bufconn)
			if nil != err {
				log.Printf("Session:%d read first request failed from proxy connection for reason:%v", sid, err)
				return
			}
			if strings.Contains(initialHTTPReq.Host, ":") {
				remoteHost, remotePort, _ = net.SplitHostPort(initialHTTPReq.Host)
			} else {
				remoteHost = initialHTTPReq.Host
				remotePort = "80"
				if strings.EqualFold(initialHTTPReq.Method, "CONNECT") {
					remotePort = "443"
				}
			}
			if strings.EqualFold(initialHTTPReq.Method, "CONNECT") {
				protocol = "https"
				if len(remotePort) == 0 {
					remotePort = "443"
				}
				if !isSocksProxy {
					conn.Write([]byte("HTTP/1.0 200 Connection established\r\n\r\n"))
					isHttpsProxy = true
				}
			} else {
				protocol = "http"
				if len(remotePort) == 0 {
					remotePort = "80"
				}
			}
		} else {
			if !isHttp11Proto {
				log.Printf("[ERROR]Can NOT handle non HTTP1.1 proto in non socks proxy mode.")
				return
			}
		}
	}

START:
	p = proxy.getProxyByHostPort(protocol, net.JoinHostPort(remoteHost, remotePort))
	if nil == p {
		log.Printf("[ERROR]No proxy found for %s:%s", remoteHost, remotePort)
		return
	}
	muxSession, err := getMuxSessionByProxy(p)
	if nil != err {
		log.Printf("Session:%d failed to get mux session for reason:%v", sid, err)
		return
	}
	stream, err := muxSession.OpenStream()
	if nil != err {
		log.Printf("Session:%d failed to open stream for reason:%v", sid, err)
		return
	}
	session.RemoteStream = stream
	//defer stream.Close()
	err = stream.Connect("tcp", net.JoinHostPort(remoteHost, remotePort))
	if nil != err {
		log.Printf("Session:%d connect failed from proxy connection for reason:%v", sid, err)
		return
	}
	go func() {
		io.Copy(conn, stream)
	}()
	if isSocksProxy || isHttpsProxy {
		wait := make(chan int)
		go func() {
			io.Copy(stream, conn)
			wait <- 1
		}()
		<-wait
	} else {
		proxyReq := initialHTTPReq
		initialHTTPReq = nil
		for {
			if nil != proxyReq {
				err = proxyReq.Write(stream)
				if nil != err {
					log.Printf("Session:%d failed to write http request for reason:%v", sid, err)
					return
				}
			}
			prevReq := proxyReq
			proxyReq, err = http.ReadRequest(bufconn)
			if nil != err {
				log.Printf("Session:%d failed to read proxy http request for reason:%v", sid, err)
				return
			}
			if nil != prevReq && prevReq.Host != proxyReq.Host {
				log.Printf("Session:%d switch to next stream since target host change from %s to %s", sid, prevReq.Host, proxyReq.Host)
				stream.Close()
				goto START
			}
		}
	}
}

func startLocalProxyServer(proxy ProxyConfig) (*net.TCPListener, error) {
	tcpaddr, err := net.ResolveTCPAddr("tcp", proxy.Local)
	if nil != err {
		log.Fatalf("[ERROR]Local server address:%s error:%v", proxy.Local, err)
		return nil, err
	}
	var lp *net.TCPListener
	lp, err = net.ListenTCP("tcp", tcpaddr)
	if nil != err {
		log.Fatalf("Can NOT listen on address:%s", proxy.Local)
		return nil, err
	}
	log.Printf("Listen on address %s", proxy.Local)
	go func() {
		for proxyServerRunning {
			conn, err := lp.AcceptTCP()
			if nil != err {
				continue
			}
			go serveProxyConn(conn, proxy)
		}
		lp.Close()
	}()
	return lp, nil
}

var runningServers []*net.TCPListener

func startLocalServers() error {
	proxyServerRunning = true
	runningServers = make([]*net.TCPListener, 0)
	for _, proxy := range GConf.Proxy {
		l, _ := startLocalProxyServer(proxy)
		if nil != l {
			runningServers = append(runningServers, l)
		}
	}
	return nil
}

func stopLocalServers() {
	proxyServerRunning = false
	for _, l := range runningServers {
		l.Close()
	}
	closeAllProxySession()
	closeAllUDPSession()
}

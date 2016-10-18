package proxy

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/fakecert"
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
	queue := event.NewEventQueue()
	connClosed := false
	session := newProxySession(sid, queue)
	defer closeProxySession(sid)

	remoteHost := ""
	remotePort := ""
	//indicate that if remote opened by event
	tryRemoteResolve := false
	socksConn, bufconn, err := socks.NewSocksConn(conn)

	socksInitProxy := func() {
		remoteAddr := net.JoinHostPort(remoteHost, remotePort)
		creq, _ := http.NewRequest("Connect", "https://"+remoteAddr, nil)
		p = proxy.findProxyByRequest(protocol, remoteHost, creq)
		if nil == p {
			conn.Close()
			return
		}
		log.Printf("Session:%d select channel:%s for %s", sid, p.Config().Name, remoteHost)
		// if p.Config().IsDirect() && net.ParseIP(remoteHost) != nil {
		// 	addr = net.JoinHostPort(remoteHost, remotePort)
		// }
		tcpOpen := &event.TCPOpenEvent{}
		tcpOpen.SetId(sid)
		tcpOpen.Addr = remoteAddr
		p.Serve(session, tcpOpen)
	}

	if nil == err {
		log.Printf("Local proxy recv %s proxy conn to %s", socksConn.Version(), socksConn.Req.Target)
		socksConn.Grant(&net.TCPAddr{
			IP: net.ParseIP("0.0.0.0"), Port: 0})

		if socksConn.Req.Target == GConf.UDPGWAddr {
			log.Printf("Handle udpgw conn for %v", socksConn.Req.Target)
			handleUDPGatewayConn(conn, proxy)
			return
		}
		conn = socksConn
		session.Hijacked = true

		remoteHost, remotePort, err = net.SplitHostPort(socksConn.Req.Target)
		if nil != err {
			log.Printf("Invalid socks target addresss:%s with reason %v", socksConn.Req.Target, err)
			return
		}
		if net.ParseIP(remoteHost) != nil && !helper.IsPrivateIP(remoteHost) && proxy.SNISniff {
			//this is a ip from local dns query
			tryRemoteResolve = true
			if remotePort == "80" {
				//we can parse http request directly
				session.Hijacked = false
			}
		} else {
			socksInitProxy()
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

	testConn := func() {
		if nil != p {
			var testConnEv event.ConnTestEvent
			testConnEv.SetId(sid)
			p.Serve(session, &testConnEv)
		}
	}

	go func() {
		for !connClosed {
			ev, err := queue.Read(1 * time.Second)
			if err != nil {
				if err != io.EOF {
					continue
				}
				return
			}
			//log.Printf("Session:%d recv event:%T", sid, ev)
			switch ev.(type) {
			case *event.NotifyEvent:
				//donothing now
			case *event.ConnCloseEvent:
				connClosed = true
				conn.Close()
				return
			case *event.TCPChunkEvent:
				conn.Write(ev.(*event.TCPChunkEvent).Content)
			case *event.HTTPResponseEvent:
				ev.(*event.HTTPResponseEvent).Write(conn)
				code := ev.(*event.HTTPResponseEvent).StatusCode
				log.Printf("Session:%d response:%d %v", ev.GetId(), code, http.StatusText(int(code)))
			default:
				log.Printf("Invalid event type:%T to process", ev)
			}
		}
	}()

	sniSniffed := true

	if tryRemoteResolve && session.Hijacked {
		sniSniffed = false
	}
	sniChunk := make([]byte, 0)
	for !connClosed {
		if nil != p {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		} else {
			conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		}
		if session.Hijacked {
			buffer := make([]byte, 8192)
			n, err := bufconn.Read(buffer)
			if nil != err {
				if helper.IsTimeoutError(err) {
					if nil != p {
						testConn()
					} else {
						socksInitProxy()
					}
					continue
				}
				if err != io.EOF && !connClosed {
					log.Printf("Session:%d read chunk failed from proxy connection:%v", sid, err)
				}
				break
			}
			chunkContent := buffer[0:n]
			if !sniSniffed {
				sniChunk = append(sniChunk, chunkContent...)
				sni, err := helper.TLSParseSNI(sniChunk)
				if err != nil {
					if err != helper.ErrTLSIncomplete {
						sniSniffed = true
						chunkContent = sniChunk
						//downgrade to use old address
						tryRemoteResolve = false
						socksInitProxy()
					} else {
						continue
					}
				} else {
					sniSniffed = true
					chunkContent = sniChunk
					log.Printf("Sniffed SNI:%s:%s for IP:%s:%s", sni, remotePort, remoteHost, remotePort)
					remoteHost = sni
					socksInitProxy()
				}
			}
			if nil == p {
				return
			}
			var chunk event.TCPChunkEvent
			chunk.SetId(sid)
			chunk.Content = chunkContent
			p.Serve(session, &chunk)
			continue
		}
		req, err := http.ReadRequest(bufconn)
		if nil != err {
			if helper.IsTimeoutError(err) {
				testConn()
				continue
			}
			if err != io.EOF && !connClosed {
				if len(remoteHost) > 0 {
					log.Printf("Session:%d read request failed from proxy connection to %s:%s for reason:%v", sid, remoteHost, remotePort, err)
				} else {
					log.Printf("Session:%d read request failed from proxy connection for reason:%v", sid, err)
				}
			}
			break
		}

		if nil == p {
			if strings.Contains(req.Host, ":") {
				remoteHost, remotePort, _ = net.SplitHostPort(req.Host)
			} else {
				remoteHost = req.Host
			}
			if strings.EqualFold(req.Method, "CONNECT") {
				protocol = "https"
				if len(remotePort) == 0 {
					remotePort = "443"
				}
			} else {
				protocol = "http"
				if len(remotePort) == 0 {
					remotePort = "80"
				}
			}
			p = proxy.findProxyByRequest(protocol, remoteHost, req)
			if nil == p {
				connClosed = true
				conn.Close()
				return
			}
			log.Printf("Session:%d select channel:%s for %s", sid, p.Config().Name, remoteHost)
		}
		reqUrl := req.URL.String()
		if strings.EqualFold(req.Method, "Connect") {
			reqUrl = req.URL.Host
		} else {
			if !strings.HasPrefix(reqUrl, "http://") && !strings.HasPrefix(reqUrl, "https://") {
				if session.SSLHijacked {
					reqUrl = "https://" + req.Host + reqUrl
				} else {
					reqUrl = "http://" + req.Host + reqUrl
				}
			}
		}
		//log.Printf("Session:%d request:%s %v %v %v", sid, req.Method, reqUrl, req.Header, req.TransferEncoding)

		req.Header.Del("Proxy-Connection")
		ev := event.NewHTTPRequestEvent(req)
		ev.SetId(sid)
		maxBody := p.Features().MaxRequestBody
		if maxBody > 0 && req.ContentLength > 0 {
			if int64(maxBody) < req.ContentLength {
				log.Printf("[ERROR]Too large request:%d for limit:%d", req.ContentLength, maxBody)
				return
			}
			ev.Headers.Del("Transfer-Encoding")
			for int64(len(ev.Content)) < req.ContentLength {
				buffer := make([]byte, 8192)
				n, err := req.Body.Read(buffer)
				if nil != err {
					break
				}
				ev.Content = append(ev.Content, buffer[0:n]...)
			}
		}

		if tryRemoteResolve && p.Config().IsDirect() && net.ParseIP(remoteHost) != nil && session.Remote == nil {
			tcpOpen := &event.TCPOpenEvent{}
			tcpOpen.SetId(sid)
			tcpOpen.Addr = net.JoinHostPort(remoteHost, remotePort)
			p.Serve(session, tcpOpen)
		}

		p.Serve(session, ev)
		if maxBody < 0 && req.ContentLength != 0 {
			for nil != req.Body {
				buffer := make([]byte, 8192)
				n, err := req.Body.Read(buffer)
				if n > 0 {
					buffer = buffer[0:n]
					var chunk event.TCPChunkEvent
					chunk.SetId(sid)
					if req.ContentLength > 0 {
						chunk.Content = buffer
					} else {
						//HTTP chunked body
						var chunkBuffer bytes.Buffer
						fmt.Fprintf(&chunkBuffer, "%x\r\n", len(buffer))
						chunkBuffer.Write(buffer)
						chunkBuffer.WriteString("\r\n")
						chunk.Content = chunkBuffer.Bytes()
					}
					p.Serve(session, &chunk)
				}
				if nil != err {
					//HTTP chunked body EOF
					if err == io.EOF && req.ContentLength < 0 {
						var eofChunk event.TCPChunkEvent
						eofChunk.SetId(sid)
						eofChunk.Content = []byte("0\r\n")
						p.Serve(session, &eofChunk)
					}
					break
				}
			}
		}
		if strings.EqualFold(req.Method, "Connect") && (session.SSLHijacked || session.Hijacked) {
			conn.Write([]byte("HTTP/1.0 200 Connection established\r\n\r\n"))
		}

		//do not parse http rquest next process,since it would upgrade to websocket/spdy/http2
		if len(req.Header.Get("Upgrade")) > 0 {
			log.Printf("Session:%d upgrade protocol to %s", sid, req.Header.Get("Upgrade"))
			session.Hijacked = true
		}
		if session.SSLHijacked {
			if tlsconn, ok := conn.(*tls.Conn); !ok {
				tlscfg, err := fakecert.TLSConfig(req.Host)
				if nil != err {
					log.Printf("[ERROR]Failed to generate fake cert for %s:%v", req.Host, err)
					return
				}
				tlsconn = tls.Server(conn, tlscfg)
				conn = tlsconn
				bufconn = bufio.NewReader(conn)
			}
		}
	}
	if nil != p {
		tcpclose := &event.ConnCloseEvent{}
		tcpclose.SetId(sid)
		p.Serve(session, tcpclose)
	}
	connClosed = true
	conn.Close()
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

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
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/local/socks"
)

var proxyServerRunning = true

var runningProxyStreamCount int64

func serveProxyConn(conn net.Conn, proxy ProxyConfig) {
	var proxyChannelName string
	protocol := "tcp"
	localConn := conn
	atomic.AddInt64(&runningProxyStreamCount, 1)
	defer localConn.Close()
	defer atomic.AddInt64(&runningProxyStreamCount, -1)

	remoteHost := ""
	remotePort := ""
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
		localConn = socksConn
		if socksConn.Req.Target == GConf.UDPGW.Addr {
			log.Printf("Handle udpgw conn for %v", socksConn.Req.Target)
			handleUDPGatewayConn(localConn, proxy)
			return
		}

		remoteHost, remotePort, err = net.SplitHostPort(socksConn.Req.Target)
		if nil != err {
			log.Printf("Invalid socks target addresss:%s with reason %v", socksConn.Req.Target, err)
			return
		}
	} else {
		if nil == bufconn {
			localConn.Close()
			return
		}
	}

	if nil == bufconn {
		bufconn = bufio.NewReader(localConn)
	}

	trySniffDomain := false
	if len(remoteHost) == 0 || (net.ParseIP(remoteHost) != nil && !helper.IsPrivateIP(remoteHost)) {
		//this is a ip from local dns query
		trySniffDomain = true
	}

	//1. sniff SNI first
	if isSocksProxy && trySniffDomain {
		sniChunkPeekSize := 512
		for {
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
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
					trySniffDomain = false
					break
				}
			} else {
				//try next round
				break
			}
		}
	}

	//2. sniff domain via http
	if trySniffDomain {
		localConn.SetReadDeadline(time.Now().Add(60 * time.Second))
		headChunk, err := bufconn.Peek(7)
		if len(headChunk) != 7 {
			if err != io.EOF {
				log.Printf("Peek:%s %d %v", string(headChunk), len(headChunk), err)
			}
			return
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
		//log.Printf("[%d]]Method:%s", sid, method)
		if isHttp11Proto {
			initialHTTPReq, err = http.ReadRequest(bufconn)
			if nil != err {
				log.Printf("Read first request failed from proxy connection for reason:%v", err)
				return
			}
			//log.Printf("Host:%s %v", initialHTTPReq.Host, initialHTTPReq.URL)
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
				if !isSocksProxy {
					localConn.Write([]byte("HTTP/1.0 200 Connection established\r\n\r\n"))
					isHttpsProxy = true
					initialHTTPReq = nil
				}
			} else {
				protocol = "http"
			}
		} else {
			if !isHttp11Proto && !isSocksProxy {
				log.Printf("[ERROR]Can NOT handle non HTTP1.1 proto in non socks proxy mode.")
				return
			}
		}
	}
START:
	if len(remoteHost) == 0 || len(remotePort) == 0 {
		log.Printf("Can NOT resolve remote host or port %s:%s", remoteHost, remotePort)
		return
	}
	proxyChannelName = proxy.getProxyChannelByHost(protocol, remoteHost)

	if len(proxyChannelName) == 0 {
		log.Printf("[ERROR]No proxy found for %s:%s", protocol, remoteHost)
		return
	}
	stream, conf, err := getMuxStreamByChannel(proxyChannelName)
	if nil != err || nil == stream {
		log.Printf("Failed to open stream for reason:%v by proxy:%s", err, proxyChannelName)
		return
	}
	defer stream.Close()
	ssid := stream.StreamID()
	log.Printf("Stream[%d] select %s for proxy to %s:%s", ssid, proxyChannelName, remoteHost, remotePort)
	err = stream.Connect("tcp", net.JoinHostPort(remoteHost, remotePort))
	if nil != err {
		log.Printf("Connect failed from proxy connection for reason:%v", err)
		return
	}

	//clear read timeout
	var zero time.Time
	localConn.SetReadDeadline(zero)
	streamReader, streamWriter := mux.GetCompressStreamReaderWriter(stream, conf.Compressor)

	go func() {
		io.Copy(localConn, streamReader)
	}()
	if isSocksProxy || isHttpsProxy {
		io.Copy(streamWriter, bufconn)
		if close, ok := streamWriter.(io.Closer); ok {
			close.Close()
		}
	} else {
		proxyReq := initialHTTPReq
		initialHTTPReq = nil
		for {
			if nil != proxyReq {
				proxyReq.Header.Del("Proxy-Connection")
				err = proxyReq.Write(streamWriter)
				if nil != err {
					log.Printf("Failed to write http request for reason:%v", err)
					return
				}
			}
			prevReq := proxyReq
			localConn.SetReadDeadline(time.Now().Add(60 * time.Second))
			proxyReq, err = http.ReadRequest(bufconn)
			if nil != err {
				if err != io.EOF && !strings.Contains(err.Error(), "use of closed network connection") {
					log.Printf("Failed to read proxy http request for reason:%v", err)
				}
				return
			}
			if nil != prevReq && prevReq.Host != proxyReq.Host {
				log.Printf("Switch to next stream since target host change from %s to %s", prevReq.Host, proxyReq.Host)
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
	//closeAllProxySession()
	closeAllUDPSession()
}

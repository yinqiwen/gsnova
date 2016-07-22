package proxy

import (
	"bufio"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/fakecert"
)

type proxyHandler struct {
	Config ProxyConfig
}

func (p *proxyHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

}

var seed uint32 = 0

func serveProxyConn(conn net.Conn, proxy ProxyConfig) {
	bufconn := bufio.NewReader(conn)
	defer conn.Close()
	var p Proxy
	sid := atomic.AddUint32(&seed, 1)
	queue := event.NewEventQueue()
	go func() {
		for {
			ev, err := queue.Read(1 * time.Second)
			if err != nil {
				if err != io.EOF {
					continue
				}
				return
			}
			//log.Printf("Session:%d recv event:%T", sid, ev)
			switch ev.(type) {
			case *event.ErrorEvent:
				err := ev.(*event.ErrorEvent)
				log.Printf("[ERROR]Session:%d recv error %d:%s", err.Code, err.Reason)
				conn.Close()
				return
			case *event.TCPCloseEvent:
				conn.Close()
				return
			case *event.TCPChunkEvent:
				conn.Write(ev.(*event.TCPChunkEvent).Content)
			case *event.HTTPResponseEvent:
				//rr := ev.(*event.HTTPResponseEvent).ToResponse(true)
				//rr.Write(connWriter)
				ev.(*event.HTTPResponseEvent).Write(conn)
				code := ev.(*event.HTTPResponseEvent).StatusCode
				log.Printf("Session:%d response:%d %v", ev.GetId(), code, http.StatusText(int(code)))
			default:
				log.Printf("Invalid event type:%T to process", ev)
			}
		}
	}()
	session := newProxySession(sid, queue)
	defer closeProxySession(sid)
	proxyName := ""
	for {
		if session.Hijacked {
			buffer := make([]byte, 8192)
			n, err := bufconn.Read(buffer)
			if nil != err {
				log.Printf("Session:%d read chunk failed from proxy connection:%v", sid, err)
				break
			}
			var chunk event.TCPChunkEvent
			chunk.SetId(sid)
			chunk.Content = buffer[0:n]
			p.Serve(session, &chunk)
			continue
		}
		req, err := http.ReadRequest(bufconn)
		if nil != err {
			log.Printf("Session:%d read request failed from proxy connection:%v", sid, err)
			break
		}
		if nil == p {
			for _, pac := range proxy.PAC {
				if pac.Match(req) {
					p = getProxyByName(pac.Remote)
					proxyName = pac.Remote
					break
				}

			}
			if nil == p {
				log.Printf("No proxy found.")
				return
			}
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
		log.Printf("[%s]Session:%d request:%s %v", proxyName, sid, req.Method, reqUrl)

		req.Header.Del("Proxy-Connection")
		ev := event.NewHTTPRequestEvent(req)
		ev.SetId(sid)
		maxBody := p.Features().MaxRequestBody
		if maxBody > 0 && req.ContentLength > 0 {
			if int64(maxBody) < req.ContentLength {
				log.Printf("[ERROR]Too large request:%d for limit:%d", req.ContentLength, maxBody)
				return
			}
			for int64(len(ev.Content)) < req.ContentLength {
				buffer := make([]byte, 8192)
				n, err := req.Body.Read(buffer)
				if nil != err {
					break
				}
				ev.Content = append(ev.Content, buffer[0:n]...)
			}
		}

		p.Serve(session, ev)
		if maxBody < 0 && req.ContentLength != 0 {
			for nil != req.Body {
				buffer := make([]byte, 8192)
				n, err := req.Body.Read(buffer)
				if nil != err {
					break
				}
				var chunk event.TCPChunkEvent
				chunk.SetId(sid)
				chunk.Content = buffer[0:n]
				p.Serve(session, &chunk)
			}
		}
		if strings.EqualFold(req.Method, "Connect") && (session.SSLHijacked || session.Hijacked) {
			conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
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
		tcpclose := &event.TCPCloseEvent{}
		tcpclose.SetId(sid)
		p.Serve(session, tcpclose)
	}
}

func startLocalProxyServer(proxy ProxyConfig) error {
	tcpaddr, err := net.ResolveTCPAddr("tcp", proxy.Local)
	if nil != err {
		log.Fatalf("[ERROR]Local server address:%s error:%v", proxy.Local, err)
		return err
	}
	var lp *net.TCPListener
	lp, err = net.ListenTCP("tcp", tcpaddr)
	if nil != err {
		log.Fatalf("Can NOT listen on address:%s", proxy.Local)
		return err
	}
	log.Printf("Listen on address %s", proxy.Local)
	go func() {
		for {
			conn, err := lp.AcceptTCP()
			if nil != err {
				continue
			}
			go serveProxyConn(conn, proxy)
		}
	}()
	return nil
}

func startLocalServers() error {
	for _, proxy := range GConf.Proxy {
		startLocalProxyServer(proxy)
	}
	return nil
}

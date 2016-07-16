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

func serveProxyConn(conn *net.TCPConn, proxy ProxyConfig) {
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
				ev.(*event.HTTPResponseEvent).Write(conn)
			default:
				log.Printf("Invalid event type:%T to process", ev)
			}
		}
	}()
	session := newProxySession(sid, queue)
	defer closeProxySession(sid)
	for {
		if session.Hijacked {
			buffer := make([]byte, 8192)
			n, err := bufconn.Read(buffer)
			if nil != err {
				queue.Close()
				break
			}
			var chunk event.TCPChunkEvent
			chunk.SetId(sid)
			chunk.Content = buffer[0:n]
			p.Serve(session, &chunk)
			continue
		}
		req, err := http.ReadRequest(bufconn)
		if nil != err && err != io.EOF {
			log.Printf("Read request failed from proxy connection:%v", err)
			break
		}
		if err != nil {
			queue.Close()
			break
		}
		log.Printf("Session:%d request:%s %v", sid, req.Method, req.URL)
		req.Header.Del("Proxy-Connection")
		if nil == p {
			for _, pac := range proxy.PAC {
				if pac.Match(req) {
					p = getProxyByName(pac.Remote)
					break
				}
			}
			if nil == p {
				log.Printf("No proxy found.")
				return
			}
		}

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
		if session.SSLHijacked && strings.EqualFold(req.Method, "Connect") {
			tlscfg, err := fakecert.TLSConfig(req.Host)
			if nil != err {
				log.Printf("[ERROR]Failed to generate fake cert for %s:%v", req.Host, err)
				return
			}
			tlsconn := tls.Server(conn, tlscfg)
			bufconn.Reset(tlsconn)
		}
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

	}
	tcpclose := &event.TCPCloseEvent{}
	tcpclose.SetId(sid)
	p.Serve(session, tcpclose)
}

func startLocalProxyServer(proxy ProxyConfig) error {
	tcpaddr, err := net.ResolveTCPAddr("tcp", proxy.Local)
	if nil != err {
		return err
	}
	var lp *net.TCPListener
	lp, err = net.ListenTCP("tcp", tcpaddr)
	if nil != err {
		log.Fatalf("Can NOT listen on address:%s", proxy.Local)
		return err
	}
	log.Printf("Listen on address %s", proxy.Local)
	for {
		conn, err := lp.AcceptTCP()
		if nil != err {
			continue
		}
		go serveProxyConn(conn, proxy)
	}
}

func startLocalServers() error {
	for _, proxy := range GConf.Proxy {
		startLocalProxyServer(proxy)
	}
	return nil
}

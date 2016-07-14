package proxy

import (
	"bufio"
	"io"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	//"time"
)

// var seed uint32 = 0

// func handleConn(conn *net.TCPConn, proxyServerType int) {
// 	sessionID := atomic.AddUint32(&seed, 1)
// 	proxy.HandleConn(sessionID, conn, proxyServerType)
// }

// func handleServer(lp *net.TCPListener, proxyServerType int) {
// 	for {
// 		conn, err := lp.AcceptTCP()
// 		if nil != err {
// 			continue
// 		}
// 		go handleConn(conn, proxyServerType)
// 	}
// }

type proxyHandler struct {
	Config ProxyConfig
}

func (p *proxyHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

}

var seed uint32 = 0

func serveProxyConn(conn *net.TCPConn, proxy ProxyConfig) {
	bufconn := bufio.NewReader(conn)
	defer conn.Close()
	p := getProxyByName(proxy.Remote)
	if nil == p {
		log.Printf("No proxy found for %s", proxy.Remote)
		return
	}
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
			log.Printf("Session:%d recv event:%T", sid, ev)
			switch ev.(type) {
			case *event.TCPCloseEvent:
				conn.Close()
				return
			case *event.TCPChunkEvent:
				conn.Write(ev.(*event.TCPChunkEvent).Content)
			case *event.HTTPResponseEvent:
				//res := ev.(*event.HTTPResponseEvent).ToResponse(false)
				ev.(*event.HTTPResponseEvent).Write(conn)
				// dump, err := httputil.DumpResponse(res, false)
				// if err != nil {
				// 	log.Printf("Failed to dump response:%v", err)
				// } else {
				// 	log.Printf("####%s %d", string(dump), ev.(*event.HTTPResponseEvent).GetContentLength())
				// 	conn.Write(dump)
				// 	conn.Write(ev.(*event.HTTPResponseEvent).Content)
				// }
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
			//if err != io.EOF {
			log.Printf("Read request failed from proxy connection:%v", err)
			//}
			break
		}
		if err != nil {
			queue.Close()
			break
		}
		req.Header.Del("Proxy-Connection")
		ev := event.NewHTTPRequestEvent(req)
		ev.SetId(sid)
		p.Serve(session, ev)
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

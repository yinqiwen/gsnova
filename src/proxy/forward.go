package proxy

import (
	"bufio"
	"event"
	"io"
	"log"
	"misc/socks"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ForwardConnection struct {
	forward_conn net.Conn
	conn_url     *url.URL
	proxyAddr    string
	forwardChan  chan int
	manager      *Forward
}

func (conn *ForwardConnection) Close() error {
	if nil != conn.forward_conn {
		conn.forward_conn.Close()
	}
	return nil
}

func (conn *ForwardConnection) initForwardConn(proxyAddr string) error {
	if nil != conn.forward_conn && conn.proxyAddr == proxyAddr {
		return nil
	}
	conn.Close()
	var err error
	conn.conn_url, err = url.Parse(conn.manager.target)
	if nil != err {
		return err
	}
	if !strings.Contains(proxyAddr, ":") {
		proxyAddr = proxyAddr + ":80"
	}
	addr := conn.conn_url.Host
	if !conn.manager.overProxy {
		addr = lookupHostPort(addr)
	}

	isSocks := strings.HasPrefix(strings.ToLower(conn.conn_url.Scheme), "socks")
	if !isSocks {
		conn.forward_conn, err = net.DialTimeout("tcp", addr, 2*time.Second)
		if nil != err {
			conn.forward_conn, err = net.DialTimeout("tcp", addr, 4*time.Second)
		}
	} else {
		proxy := &socks.Proxy{Addr: conn.conn_url.Host}
		if nil != conn.conn_url.User {
			proxy.Username = conn.conn_url.User.Username()
			proxy.Password, _ = conn.conn_url.User.Password()
		}
		conn.forward_conn, err = proxy.Dial("tcp", proxyAddr)
	}
	if nil != err {
		log.Printf("Failed to dial address:%s for reason:%s\n", addr, err.Error())
		return err
	} else {
		conn.proxyAddr = proxyAddr
	}
	return nil
}

func (conn *ForwardConnection) GetConnectionManager() RemoteConnectionManager {
	return conn.manager
}

func (conn *ForwardConnection) writeHttpRequest(req *http.Request) error {
	var err error
	index := 0
	for {
		if conn.manager.overProxy {
			err = req.WriteProxy(conn.forward_conn)
		} else {
			err = req.Write(conn.forward_conn)
		}

		if nil != err {
			log.Printf("Resend request since error:%v occured.\n", err)
			conn.Close()
			conn.initForwardConn(req.Host)
		} else {
			return nil
		}
		index++
		if index == 2 {
			return err
		}
	}
	return nil
}

func (auto *ForwardConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	f := func(local, remote net.Conn) {
		io.Copy(remote, local)
		auto.forwardChan <- 1
	}
	switch ev.GetType() {
	case event.HTTP_REQUEST_EVENT_TYPE:
		req := ev.(*event.HTTPRequestEvent)
		err = auto.initForwardConn(req.RawReq.Host)
		if nil == auto.forward_conn {
			log.Printf("Failed to connect forward proxy.\n")
			//conn.LocalRawConn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
			//conn.LocalRawConn.Close()
			return err, nil
		}
		if conn.Type == HTTPS_TUNNEL {
			log.Printf("Session[%d]Request URL:%s %s\n", ev.GetHash(), req.RawReq.Method, req.RawReq.RequestURI)
			conn.LocalRawConn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
			go f(conn.LocalRawConn, auto.forward_conn)
			go f(auto.forward_conn, conn.LocalRawConn)
			<-auto.forwardChan
			<-auto.forwardChan
			auto.Close()
			conn.State = STATE_SESSION_CLOSE
		} else {
			log.Printf("Session[%d]Request URL:%s %s\n", ev.GetHash(), req.RawReq.Method, req.RawReq.RequestURI)
			if !auto.manager.overProxy && needInjectCRLF(req.RawReq.Host) {
				auto.forward_conn.Write([]byte("\r\n\r\n"))
			}
			err := auto.writeHttpRequest(req.RawReq)
			if nil != err {
				return err, nil
			}
			resp, err := http.ReadResponse(bufio.NewReader(auto.forward_conn), req.RawReq)
			if err != nil {
				return err, nil
			}
			err = resp.Write(conn.LocalRawConn)
			if nil != err || resp.Close || req.RawReq.Close {
				conn.LocalRawConn.Close()
				auto.Close()
				conn.State = STATE_SESSION_CLOSE
			} else {
				conn.State = STATE_RECV_HTTP
			}
		}

	default:
	}
	return nil, nil
}

type Forward struct {
	target    string
	overProxy bool
}

func (manager *Forward) GetName() string {
	return FORWARD_NAME + manager.target
}

func (manager *Forward) GetArg() string {
	return manager.target
}
func (manager *Forward) RecycleRemoteConnection(conn RemoteConnection) {

}

func (manager *Forward) GetRemoteConnection(ev event.Event) (RemoteConnection, error) {
	g := new(ForwardConnection)
	g.manager = manager
	g.Close()
	return g, nil
}

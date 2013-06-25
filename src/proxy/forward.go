package proxy

import (
	"bufio"
	"bytes"
	"common"
	"event"
	"fmt"
	"io"
	"log"
	"misc/socks"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
	"util"
)

var CRLFs = []byte("\r\n\r\n")

var total_forwared_conn_num int32
var total_forwared_routine_num int32

type ForwardConnection struct {
	forward_conn     net.Conn
	buf_forward_conn *bufio.Reader
	conn_url         *url.URL
	proxyAddr        string
	forwardChan      chan int
	manager          *Forward
	use_sys_dns      bool
	closed           bool

	checkChannel chan int
}

func (conn *ForwardConnection) Close() error {
	if nil != conn.forward_conn {
		conn.forward_conn.Close()
		conn.forward_conn = nil
	}
	conn.closed = true
	return nil
}

func (conn *ForwardConnection) IsClosed() bool {
	if nil != conn.forward_conn && !conn.closed {
		return util.IsDeadConnection(conn.forward_conn)
	}
	return true
}

func createDirectForwardConn(hostport string) (net.Conn, error) {
	addr, _ := lookupAvailableAddress(hostport)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if nil != err {
		conn, err = net.DialTimeout("tcp", addr, 4*time.Second)
	}
	if nil != err {
		expireBlockVerifyCache(addr)
	}
	return conn, err
}

func (conn *ForwardConnection) dialRemote(addr string, lookup_trusted_dns bool) (net.Conn, error) {
	timeout := 10 * time.Second
	//	if !lookup_trusted_dns {
	//		if exist := getDomainCRLFAttr(addr); exist {
	//			conn.try_inject_crlf = true
	//			lookup_trusted_dns = true
	//		}
	//	}
	orgin_addr := addr
	addr = getAddressMapping(orgin_addr)
	if lookup_trusted_dns {
		if newaddr, success := lookupAvailableAddress(addr); !success {
			return nil, fmt.Errorf("No available IP found for %s", orgin_addr)
		} else {
			log.Printf("Found %s for %s\n", newaddr, addr)
			addr = newaddr
		}
	}
	c, err := net.DialTimeout("tcp", addr, timeout)
	if nil != err && !lookup_trusted_dns {
		if tmp, success := lookupAvailableAddress(addr); success {
			//conn.try_inject_crlf = true
			c, err = net.DialTimeout("tcp", tmp, timeout)
		}
	}
	return c, err
}

func (conn *ForwardConnection) initForwardConn(proxyAddr string, isHttps bool) error {
	if !strings.Contains(proxyAddr, ":") {
		proxyAddr = proxyAddr + ":80"
	}

	if nil != conn.forward_conn && conn.proxyAddr == proxyAddr {
		if !util.IsDeadConnection(conn.forward_conn) {
			return nil
		}
	}
	conn.Close()
	var err error
	conn.conn_url, err = url.Parse(conn.manager.target)
	if nil != err {
		return err
	}

	addr := conn.conn_url.Host
	lookup_trusted_dns := false
	if !conn.manager.overProxy {
		if isHttps || (conn.manager.inject_crlf) {
			lookup_trusted_dns = true
		}
	}

	if conn.use_sys_dns {
		lookup_trusted_dns = false
	}

	isSocks := strings.HasPrefix(strings.ToLower(conn.conn_url.Scheme), "socks")
	if !isSocks {
		conn.forward_conn, err = conn.dialRemote(addr, lookup_trusted_dns)
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
		conn.Close()
		return err
	} else {
		conn.proxyAddr = proxyAddr
	}
	if nil == conn.forwardChan {
		conn.forwardChan = make(chan int, 2)
	}
	conn.buf_forward_conn = bufio.NewReader(conn.forward_conn)
	conn.closed = false
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
			conn.initForwardConn(req.Host, strings.EqualFold(req.Method, "Connect"))
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
		n, err := io.Copy(remote, local)
		if nil != err {
			local.Close()
			remote.Close()
		}
		auto.forwardChan <- int(n)
	}
	//L:
	auto.closed = false
	switch ev.GetType() {
	case event.HTTP_REQUEST_EVENT_TYPE:
		req := ev.(*event.HTTPRequestEvent)
		addr := req.RawReq.Host
		if !strings.Contains(addr, ":") {
			if conn.Type == HTTPS_TUNNEL {
				addr = net.JoinHostPort(addr, "443")
			} else {
				addr = net.JoinHostPort(addr, "80")
			}
		}
		err = auto.initForwardConn(addr, conn.Type == HTTPS_TUNNEL)
		if nil != err {
			log.Printf("Failed to connect forward address for %s.\n", addr)
			return err, nil
		}
		if conn.Type == HTTPS_TUNNEL {
			log.Printf("Session[%d]Request %s\n", req.GetHash(), util.GetURLString(req.RawReq, true))
			if !auto.manager.overProxy || strings.HasPrefix(auto.conn_url.Scheme, "socks") {
				conn.LocalRawConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
			} else {
				auto.writeHttpRequest(req.RawReq)
			}
			go f(conn.LocalRawConn, auto.forward_conn)
			go f(auto.forward_conn, conn.LocalRawConn)
			atomic.AddInt32(&total_forwared_routine_num, 2)
			<-auto.forwardChan
			<-auto.forwardChan
			atomic.AddInt32(&total_forwared_routine_num, -2)
			auto.Close()
			conn.State = STATE_SESSION_CLOSE
		} else {
			log.Printf("Session[%d]Request %s\n", req.GetHash(), util.GetURLString(req.RawReq, true))
			if auto.manager.inject_crlf {
				log.Printf("Session[%d]Inject CRLF for %s", ev.GetHash(), req.RawReq.Host)
				auto.forward_conn.Write(CRLFs)
			}

			err := auto.writeHttpRequest(req.RawReq)
			if nil != err {
				return err, nil
			}
			if common.DebugEnable {
				var tmp bytes.Buffer
				req.RawReq.Write(&tmp)
				log.Printf("Session[%d]Send request \n%s\n", ev.GetHash(), tmp.String())
			}
			resp, err := http.ReadResponse(auto.buf_forward_conn, req.RawReq)
			if err != nil {
				log.Printf("Session[%d]Recv response with error %v\n", ev.GetHash(), err)
				return err, nil
			}
			//log.Printf("Session[%d]Recv response  %v\n", ev.GetHash(), resp)
			err = resp.Write(conn.LocalRawConn)
			resp.Body.Close()

			if common.DebugEnable {
				var tmp bytes.Buffer
				resp.Write(&tmp)
				log.Printf("Session[%d]Recv response \n%s\n", ev.GetHash(), tmp.String())
			}
			if nil != err || !util.IsResponseKeepAlive(resp) || !util.IsRequestKeepAlive(req.RawReq) {
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
	target       string
	overProxy    bool
	inject_crlf  bool
	inject_range bool
}

func (manager *Forward) GetName() string {
	return FORWARD_NAME + manager.target
}

func (manager *Forward) GetArg() string {
	return manager.target
}
func (manager *Forward) RecycleRemoteConnection(conn RemoteConnection) {
	atomic.AddInt32(&total_forwared_conn_num, -1)
}

func (manager *Forward) GetRemoteConnection(ev event.Event, attrs map[string]string) (RemoteConnection, error) {
	g := new(ForwardConnection)
	g.manager = manager
	g.Close()
	if containsAttr(attrs, ATTR_CRLF_INJECT) {
		//manager.inject_crlf = true
	}
	if containsAttr(attrs, ATTR_RANGE) {
		manager.inject_range = true
	}
	if containsAttr(attrs, ATTR_SYS_DNS) {
		g.use_sys_dns = true
	}
	atomic.AddInt32(&total_forwared_conn_num, 1)
	return g, nil
}

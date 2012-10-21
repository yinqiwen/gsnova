package proxy

import (
	"bufio"
	"common"
	"errors"
	"event"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	HTTP_TUNNEL  = 1
	HTTPS_TUNNEL = 2
	SOCKS_TUNNEL = 3

	STATE_RECV_HTTP       = 1
	STATE_RECV_HTTP_CHUNK = 2
	STATE_RECV_TCP        = 3
	STATE_SESSION_CLOSE   = 4

	GAE_NAME                 = "GAE"
	C4_NAME                  = "C4"
	GOOGLE_NAME              = "Google"
	GOOGLE_HTTP_NAME         = "GoogleHttp"
	GOOGLE_HTTPS_NAME        = "GoogleHttps"
	GOOGLE_HTTPS_DIRECT_NAME = "GoogleHttpsDirect"
	FORWARD_NAME             = "Forward"
	SSH_NAME                 = "SSH"
	DIRECT_NAME              = "Direct"
	DEFAULT_NAME             = "Default"

	ATTR_REDIRECT_HTTPS = "RedirectHttps"
	ATTR_CRLF_INJECT    = "CRLF"
	ATTR_DIRECT         = "Direct"

	MODE_HTTP    = "http"
	MODE_HTTPS   = "httpS"
	MODE_RSOCKET = "rsocket"
	MODE_XMPP    = "xmpp"
)

type RemoteConnection interface {
	Request(conn *SessionConnection, ev event.Event) (err error, res event.Event)
	GetConnectionManager() RemoteConnectionManager
	Close() error
}

type RemoteConnectionManager interface {
	GetRemoteConnection(ev event.Event, attrs map[string]string) (RemoteConnection, error)
	RecycleRemoteConnection(conn RemoteConnection)
	GetName() string
}

type SessionConnection struct {
	SessionID       uint32
	LocalBufferConn *bufio.Reader
	LocalRawConn    net.Conn
	RemoteConn      RemoteConnection
	State           uint32
	Type            uint32
}

func newSessionConnection(sessionId uint32, conn net.Conn, reader *bufio.Reader) *SessionConnection {
	session_conn := new(SessionConnection)
	session_conn.LocalRawConn = conn
	session_conn.LocalBufferConn = reader
	session_conn.SessionID = sessionId
	session_conn.State = STATE_RECV_HTTP
	session_conn.Type = HTTP_TUNNEL

	return session_conn
}

func (session *SessionConnection) tryProxy(proxies []RemoteConnectionManager, attrs map[string]string, ev *event.HTTPRequestEvent) (err error) {
	for _, proxy := range proxies {
		session.RemoteConn, err = proxy.GetRemoteConnection(ev, attrs)
		if nil == err {
			err, _ = session.RemoteConn.Request(session, ev)
		}
		if nil == err {
			return nil
		} else {
			log.Printf("Session[%d][WARN]Failed to request proxy event for reason:%v", session.SessionID, err)
		}
	}
	return errors.New("No proxy found")
}

func (session *SessionConnection) processHttpEvent(ev *event.HTTPRequestEvent) error {
	ev.SetHash(session.SessionID)
	proxies, attrs := SelectProxy(ev.RawReq, session.LocalRawConn, session.Type == HTTPS_TUNNEL)
	if nil == proxies {
		session.State = STATE_SESSION_CLOSE
		return nil
	}
	var err error
	if nil == session.RemoteConn {
		err = session.tryProxy(proxies, attrs, ev)
	} else {
		rmanager := session.RemoteConn.GetConnectionManager()
		matched := false
		for _, proxy := range proxies {
			if rmanager.GetName() == proxy.GetName() {
				matched = true
				break
			}
		}
		if !matched {
			session.RemoteConn.Close()
			err = session.tryProxy(proxies, attrs, ev)
		} else {
			err, _ = session.RemoteConn.Request(session, ev)
		}
	}

	if nil != err {
		log.Printf("Session[%d]Process error:%v", session.SessionID, err)
		session.LocalRawConn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
		session.LocalRawConn.Close()
	}
	return nil
}

func (session *SessionConnection) processHttpChunkEvent(ev *event.HTTPChunkEvent) error {
	ev.SetHash(session.SessionID)
	if nil != session.RemoteConn {
		session.RemoteConn.Request(session, ev)
	}
	return nil
}

func (session *SessionConnection) process() error {
	close_session := func() {
		session.LocalRawConn.Close()
		if nil != session.RemoteConn {
			session.RemoteConn.Close()
		}
		session.State = STATE_SESSION_CLOSE
	}

	switch session.State {
	case STATE_RECV_HTTP:
		session.LocalRawConn.SetReadDeadline(time.Now().Add(common.KeepAliveTimeout * time.Second))
		req, err := http.ReadRequest(session.LocalBufferConn)
		if nil == err {
			var zero time.Time
			session.LocalRawConn.SetReadDeadline(zero)
			var rev event.HTTPRequestEvent
			rev.FromRequest(req)
			rev.SetHash(session.SessionID)
			err = session.processHttpEvent(&rev)
		}
		if nil != err {
			operr, ok := err.(*net.OpError)
			if err != io.EOF || (ok && (operr.Timeout() || operr.Temporary())) {
				err = nil
			}
			if err != nil {
				log.Printf("Session[%d]Failed to read http request:%v\n", session.SessionID, err)
			}
			close_session()
		}
	case STATE_RECV_HTTP_CHUNK:
		buf := make([]byte, 8192)
		n, err := session.LocalBufferConn.Read(buf)
		if nil == err {
			rev := new(event.HTTPChunkEvent)
			rev.Content = buf[0:n]
			err = session.processHttpChunkEvent(rev)
		}
		if nil != err {
			operr, ok := err.(*net.OpError)
			if ok && (operr.Timeout() || operr.Temporary()) {
				log.Printf("Timeout to read\n")
				return nil
			}
			if err != io.EOF {
				log.Printf("Session[%d]Failed to read http chunk:%v %T\n", session.SessionID, err, err)
			}
			close_session()
		}
	case STATE_RECV_TCP:

	}
	return nil
}

func HandleConn(sessionId uint32, conn net.Conn) {
	bufreader := bufio.NewReader(conn)
	b, err := bufreader.Peek(7)
	if nil != err {
		if err != io.EOF {
			log.Printf("Failed to peek data:%s\n", err.Error())
		}
		conn.Close()
		return
	}
	session := newSessionConnection(sessionId, conn, bufreader)
	if strings.EqualFold(string(b), "Connect") {
		session.Type = HTTPS_TUNNEL
	} else if b[0] == byte(4) || b[0] == byte(5) {
		session.Type = SOCKS_TUNNEL
	} else {
		session.Type = HTTP_TUNNEL
	}
	for session.State != STATE_SESSION_CLOSE {
		err := session.process()
		if nil != err {
			return
		}
	}
	if nil != session.RemoteConn {
		session.RemoteConn.GetConnectionManager().RecycleRemoteConnection(session.RemoteConn)
	}
}

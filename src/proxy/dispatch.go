package proxy

import (
	"bufio"
	"errors"
	"event"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

const (
	HTTP_TUNNEL  = 1
	HTTPS_TUNNEL = 2
	SOCKS_TUNNEL = 3

	STATE_RECV_HTTP       = 1
	STATE_RECV_HTTP_CHUNK = 2
	STATE_RECV_TCP        = 3
	STATE_SESSION_CLOSE   = 4

	GAE_NAME          = "GAE"
	C4_NAME           = "C4"
	GOOGLE_NAME       = "Google"
	GOOGLE_HTTP_NAME  = "GoogleHttp"
	GOOGLE_HTTPS_NAME = "GoogleHttps"
	FORWARD_NAME      = "Forward"
	AUTOHOST_NAME     = "AutoHost"
	DIRECT_NAME       = "Direct"
)

type RemoteConnection interface {
	Request(conn *SessionConnection, ev event.Event) (err error, res event.Event)
	GetConnectionManager() RemoteConnectionManager
	Close() error
}

type RemoteConnectionManager interface {
	GetRemoteConnection(ev event.Event) (RemoteConnection, error)
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

func (session *SessionConnection) processHttpEvent(ev *event.HTTPRequestEvent) error {
	ev.SetHash(session.SessionID)
	proxy, exist := SelectProxy(ev.RawReq)
	if !exist {
		return errors.New("No proxy found")
	}
	var err error
	if nil == session.RemoteConn {
		session.RemoteConn, err = proxy.GetRemoteConnection(ev)
	} else {
		rmanager := session.RemoteConn.GetConnectionManager()
		if rmanager.GetName() != proxy.GetName() {
			session.RemoteConn.Close()
			session.RemoteConn, err = proxy.GetRemoteConnection(ev)
		}
	}

	err, _ = session.RemoteConn.Request(session, ev)
	if nil == err {

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
	switch session.State {
	case STATE_RECV_HTTP:
		req, err := http.ReadRequest(session.LocalBufferConn)
		if nil == err {
			var rev event.HTTPRequestEvent
			rev.FromRequest(req)
			rev.SetHash(session.SessionID)
			err = session.processHttpEvent(&rev)
		}
		if nil != err {
			if err != io.EOF {
				log.Printf("Session[%d]Failed to read http request:%v\n", session.SessionID, err)
			}
			session.LocalRawConn.Close()
			session.State = STATE_SESSION_CLOSE
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
			if err != io.EOF {
				log.Printf("Session[%d]Failed to read http chunk:%v\n", session.SessionID, err)
			}
			session.LocalRawConn.Close()
			session.State = STATE_SESSION_CLOSE
		}
	case STATE_RECV_TCP:

	}
	return nil
}

func HandleConn(sessionId uint32, conn net.Conn) {
	bufreader := bufio.NewReader(conn)
	b, err := bufreader.Peek(7)
	if nil != err {
		log.Printf("Failed to peek data:%s\n", err.Error())
		conn.Close()
		return
	}
	session := newSessionConnection(sessionId, conn, bufreader)
	//log.Printf("First str:%s\n", string(b))
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

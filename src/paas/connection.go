package paas

import (
	"bufio"
	"errors"
	"event"
	"io"
	"log"
	"net"
	"net/http"
)

const (
	HTTP_TUNNEL  = 1
	HTTPS_TUNNEL = 2
	SOCKS_TUNNEL = 3

	STATE_RECV_HTTP       = 1
	STATE_RECV_HTTP_CHUNK = 2
	STATE_RECV_TCP        = 3
	STATE_SESSION_CLOSE   = 4

	GAE_NAME = "GAE"
	C4_NAME  = "C4"
)

type RemoteConnection interface {
	Request(conn *SessionConnection, ev event.Event) (err error, res event.Event)
}

type RemoteConnectionManager interface {
	GetRemoteConnection(ev event.Event) (RemoteConnection, error)
	GetName() string
	Init() error
}

type SessionConnection struct {
	SessionID       int32
	LocalBufferConn *bufio.Reader
	LocalRawConn    net.Conn
	RemoteConn      RemoteConnection
	State           uint32
	Type            uint32
}

func newSessionConnection(sessionId int32, conn net.Conn, reader *bufio.Reader) *SessionConnection {
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
	session.RemoteConn, err = proxy.GetRemoteConnection(ev)
	if nil != err {
		return err
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
			session.processHttpEvent(&rev)
		} else {
			if err != io.EOF {
				log.Printf("Failed to read http request:%s\n", err.Error())
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
		}
	case STATE_RECV_TCP:

	}
	return nil
}

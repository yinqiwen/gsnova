package proxy

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/mux"
)

var sessions map[uint32]*ProxySession = make(map[uint32]*ProxySession)
var sessionMutex sync.Mutex
var sessionNotExist error

type ProxySession struct {
	id           uint32
	LocalConn    net.Conn
	RemoteStream mux.MuxStream
	Addr         string
	Proxy        Proxy
	createTime   time.Time
}

func (s *ProxySession) Close() error {
	if nil != s.RemoteStream {
		s.RemoteStream.Close()
		s.RemoteStream = nil
	}
	if nil != s.LocalConn {
		s.LocalConn.Close()
		s.LocalConn = nil
	}
	return nil
}

func getProxySession(sid uint32) *ProxySession {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	s, exist := sessions[sid]
	if exist {
		return s
	}
	return nil
}

func dumpProxySessions(w io.Writer) {
	fmt.Fprintf(w, "ProxySessions Dump:\n")
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	for _, s := range sessions {
		fmt.Fprintf(w, "Session[%d]:addr=%s,age=%v\n", s.id, s.Addr, time.Now().Sub(s.createTime))
	}
}

func newProxySession(sid uint32, conn net.Conn) *ProxySession {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	s := new(ProxySession)
	s.id = sid
	s.LocalConn = conn
	s.createTime = time.Now()
	sessions[s.id] = s
	return s
}

func closeProxySession(sid uint32) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	s, exist := sessions[sid]
	if exist {
		s.Close()
		delete(sessions, sid)
	}
}

func closeAllProxySession() {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	for id, s := range sessions {
		s.Close()
		delete(sessions, id)
	}
}

func getProxySessionSize() int {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	return len(sessions)
}

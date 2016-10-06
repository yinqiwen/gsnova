package proxy

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
)

var sessions map[uint32]*ProxySession = make(map[uint32]*ProxySession)
var sessionMutex sync.Mutex
var sessionNotExist error

type ProxySession struct {
	id          uint32
	queue       *event.EventQueue
	Remote      *RemoteChannel
	Hijacked    bool
	SSLHijacked bool
	createTime  time.Time
}

func (s *ProxySession) SetRemoteChannel(r *RemoteChannel) {
	if nil == s.Remote && nil != r {
		r.updateActiveSessionNum(1)
	}
	s.Remote = r
}

func (s *ProxySession) handle(ev event.Event) error {
	if nil != s.queue {
		s.queue.Publish(ev, 5*time.Second)
	}
	return nil
}

func (s *ProxySession) Close() error {
	closeEv := &event.ConnCloseEvent{}
	closeEv.SetId(s.id)
	s.handle(closeEv)
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
		if nil != s.Remote {
			fmt.Fprintf(w, "Session[%d]:proxy=%s[%d],age=%v\n", s.id, s.Remote.Addr, s.Remote.Index, time.Now().Sub(s.createTime))
		} else {
			fmt.Fprintf(w, "Session[%d]:nil remote,age=%v\n", s.id, time.Now().Sub(s.createTime))
			//delete(sessions, s.id)
		}
	}
}

func newProxySession(sid uint32, queue *event.EventQueue) *ProxySession {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	s := new(ProxySession)
	s.id = sid
	s.queue = queue
	s.createTime = time.Now()
	sessions[s.id] = s
	return s
}

func newRandomSession() *ProxySession {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return newProxySession(uint32(r.Int31()), nil)
}

func closeProxySession(sid uint32) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	s, exist := sessions[sid]
	if exist {
		if nil != s && nil != s.Remote {
			s.Remote.updateActiveSessionNum(-1)
		}
		delete(sessions, sid)
	}
}

func closeAllProxySession() {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	for id, s := range sessions {
		if nil != s && nil != s.Remote {
			s.Remote.updateActiveSessionNum(-1)
		}
		delete(sessions, id)
	}
}

func getProxySessionSize() int {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	return len(sessions)
}

func HandleEvent(ev event.Event) error {
	session := getProxySession(ev.GetId())
	if nil == session {
		switch ev.(type) {
		case *event.ConnCloseEvent:
		case *event.NotifyEvent:
		case *event.HeartBeatEvent:
		default:
			log.Printf("No session:%d found for %T", ev.GetId(), ev)
		}
		return sessionNotExist
	}
	return session.handle(ev)
}

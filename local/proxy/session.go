package proxy

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
)

// import "net"

var sessions map[uint32]*ProxySession = make(map[uint32]*ProxySession)
var sessionMutex sync.Mutex
var sessionNotExist error

type ProxySession struct {
	id          uint32
	queue       *event.EventQueue
	Remote      *RemoteChannel
	Hijacked    bool
	SSLHijacked bool
}

func (s *ProxySession) handle(ev event.Event) error {
	if nil != s.queue {
		s.queue.Publish(ev, 5*time.Second)
	}
	return nil
}

func (s *ProxySession) Close() error {
	closeEv := &event.TCPCloseEvent{}
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

func newProxySession(sid uint32, queue *event.EventQueue) *ProxySession {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	s := new(ProxySession)
	s.id = sid
	s.queue = queue
	sessions[s.id] = s
	//log.Printf("Create proxy session:%d", sid)
	return s
}

func newRandomSession() *ProxySession {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return newProxySession(uint32(r.Int31()), nil)
}

func closeProxySession(sid uint32) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	delete(sessions, sid)
	//log.Printf("Close proxy session:%d, %d left", sid, len(sessions))
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
		case *event.TCPCloseEvent:
		case *event.NotifyEvent:
		default:
			log.Printf("No session:%d found for %T", ev.GetId(), ev)
		}
		return sessionNotExist
	}
	return session.handle(ev)
}

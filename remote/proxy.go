package remote

import (
	//"fmt"
	"bytes"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
)

var proxySessionMap map[SessionId]*ProxySession = make(map[SessionId]*ProxySession)
var sessionMutex sync.Mutex

type ConnContex struct {
	User  string
	Index int
}

type SessionId struct {
	User      string
	Id        uint32
	ConnIndex int
}

type ProxySession struct {
	Id         SessionId
	CreateTime time.Time
	conn       net.Conn
	addr       string
	ch         chan event.Event
}

func GetsessionTableSize() int {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	return len(proxySessionMap)
}

func GetProxySessionByEvent(user string, idx int, ev event.Event) *ProxySession {
	sid := SessionId{user, ev.GetId(), idx}
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	if p, exist := proxySessionMap[sid]; exist {
		return p
	}
	createIfMissing := false
	switch ev.(type) {
	case *event.TCPOpenEvent:
		createIfMissing = true
	case *event.HTTPRequestEvent:
		createIfMissing = true
	}
	if !createIfMissing {
		return nil
	}
	p := new(ProxySession)
	p.Id = sid
	p.CreateTime = time.Now()
	p.ch = make(chan event.Event, 10)
	go p.processEvents()
	proxySessionMap[sid] = p
	return p
}

func removeProxySession(s *ProxySession) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	_, exist := proxySessionMap[s.Id]
	if exist {
		delete(proxySessionMap, s.Id)
		s.ch <- nil
		close(s.ch)
		log.Printf("Remove sesion:%d, %d left", s.Id.Id, len(proxySessionMap))
	}
}
func sessionExist(sid SessionId) bool {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	_, exist := proxySessionMap[sid]
	return exist
}

func removeProxySessionsByUser(user string) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	for k, s := range proxySessionMap {
		if k.User == user {
			s.close()
			delete(proxySessionMap, k)
		}
	}
}

func (p *ProxySession) publish(ev event.Event) {
	ev.SetId(p.Id.Id)
	start := time.Now()
	for {
		queue := GetEventQueue(p.Id.User, p.Id.ConnIndex, false)
		if nil != queue {
			queue.Publish(ev)
			return
		}
		if time.Now().After(start.Add(5 * time.Second)) {
			log.Printf("No avaliable connection to write event")
			p.close()
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
}

func (p *ProxySession) close() error {
	c := p.conn
	if nil != c {
		log.Printf("Session[%s:%d] close connection to %s", p.Id.User, p.Id.Id, p.addr)
		c.Close()
		p.conn = nil
		p.addr = ""
	}
	return nil
}

func (p *ProxySession) initialClose() {
	ev := &event.TCPCloseEvent{}
	p.publish(ev)
	p.close()
	removeProxySession(p)
}

func (p *ProxySession) processEvents() {
	for {
		ev := <-p.ch
		if nil != ev {
			p.handle(ev)
		} else {
			break
		}
	}
}

func (p *ProxySession) open(to string) error {
	if p.conn != nil && to == p.addr {
		return nil
	}
	p.close()
	log.Printf("Session[%s:%d] open connection to %s.", p.Id.User, p.Id.Id, to)
	c, err := net.DialTimeout("tcp", to, 5*time.Second)
	if nil != err {
		ev := &event.TCPCloseEvent{}
		p.publish(ev)
		log.Printf("Failed to connect %s for reason:%v", to, err)
		return err
	}
	p.conn = c
	p.addr = to
	go p.readTCP()
	return nil
}

func (p *ProxySession) write(b []byte) (int, error) {
	if p.conn == nil {
		log.Printf("Session[%s:%d] have no established connection to %s.", p.Id.User, p.Id.Id, p.addr)
		p.initialClose()
		return 0, nil
	}
	n, err := p.conn.Write(b)
	if nil != err {
		p.initialClose()
	}
	return n, err
}

func (p *ProxySession) readTCP() error {
	for {
		conn := p.conn
		if nil == conn {
			return nil
		}
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		b := make([]byte, 8192)
		n, err := conn.Read(b)
		if n > 0 {
			ev := &event.TCPChunkEvent{Content: b[0:n]}
			p.publish(ev)
		}
		if nil != err {
			p.initialClose()
			return err
		}

	}
	return nil
}
func (p *ProxySession) offer(ev event.Event) {
	p.ch <- ev
}

func (p *ProxySession) handle(ev event.Event) error {
	switch ev.(type) {
	case *event.TCPOpenEvent:
		return p.open(ev.(*event.TCPOpenEvent).Addr)
	case *event.TCPCloseEvent:
		p.close()
		removeProxySession(p)
	case *event.TCPChunkEvent:
		p.write(ev.(*event.TCPChunkEvent).Content)
	case *event.HTTPRequestEvent:
		req := ev.(*event.HTTPRequestEvent)
		addr := req.Headers.Get("Host")
		if !strings.Contains(addr, ":") {
			if !strings.EqualFold("Connect", req.Method) {
				addr = addr + ":80"
			} else {
				addr = addr + ":443"
			}
		}
		log.Printf("Session[%d] %s %s", ev.GetId(), req.Method, req.URL)
		err := p.open(addr)
		if nil != err {
			return err
		}
		content := req.HTTPEncode()
		_, err = p.write(content)
		return err
	default:
		log.Printf("Invalid event type:%T to process", ev)
	}
	return nil
}

func HandleRequestBuffer(reqbuf *bytes.Buffer, ctx *ConnContex) ([]event.Event, error) {
	ress := make([]event.Event, 0)
	for reqbuf.Len() > 0 {
		err, ev := event.DecodeEvent(reqbuf)
		if nil != err {
			log.Printf("Failed to decode event for reason:%v", err)
			return ress, err
		}
		if auth, ok := ev.(*event.AuthEvent); ok {
			if len(ctx.User) == 0 {
				if !ServerConf.VerifyUser(auth.User) {
					var authres event.ErrorEvent
					authres.Code = event.ErrAuthFailed
					authres.SetId(ev.GetId())
					ress = append(ress, &authres)
					return ress, nil
				}
				authedUser := auth.User
				authedUser = authedUser + "@" + auth.Mac
				log.Printf("Recv connection:%d from user:%s", auth.Index, authedUser)
				if auth.Index < 0 {
					removeProxySessionsByUser(authedUser)
					CloseUserEventQueue(authedUser)
					var authres event.ErrorEvent
					authres.Code = 0
					authres.SetId(ev.GetId())
					ress = append(ress, &authres)
					continue
				}
				ctx.User = authedUser + "@" + ev.(*event.AuthEvent).Mac
				ctx.Index = int(auth.Index)
			} else {
				return nil, fmt.Errorf("Duplicate auth event in same connection")
			}
			continue
		} else {
			if len(ctx.User) == 0 {
				return nil, fmt.Errorf("Auth event MUST be first event.")
			}
		}
		session := GetProxySessionByEvent(ctx.User, ctx.Index, ev)
		if nil != session {
			session.offer(ev)
		} else {
			if _, ok := ev.(*event.TCPCloseEvent); !ok {
				log.Printf("No session:%d found for event %T", ev.GetId(), ev)
			}
		}
	}
	return ress, nil
}

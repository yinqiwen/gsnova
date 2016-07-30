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

type ConnId struct {
	User      string
	ConnIndex int
	RunId     int64
}

type ConnContex struct {
	ConnId
	IV uint64
}

type SessionId struct {
	ConnId
	Id uint32
}

type ProxySession struct {
	Id         SessionId
	CreateTime time.Time
	conn       net.Conn
	addr       string
	network    string
	ch         chan event.Event
}

func GetSessionTableSize() int {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	return len(proxySessionMap)
}

func getProxySessionByEvent(cid ConnId, ev event.Event) *ProxySession {
	sid := SessionId{cid, ev.GetId()}
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
	case *event.UDPEvent:
		createIfMissing = true
	}
	if !createIfMissing {
		return nil
	}
	p := new(ProxySession)
	p.Id = sid
	p.CreateTime = time.Now()
	p.ch = make(chan event.Event, 100)
	go p.processEvents()
	proxySessionMap[sid] = p
	return p
}

func destroyProxySession(s *ProxySession) {
	delete(proxySessionMap, s.Id)
	s.ch <- nil
	close(s.ch)
}

func removeProxySession(s *ProxySession) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	_, exist := proxySessionMap[s.Id]
	if exist {
		destroyProxySession(s)
		//log.Printf("Remove sesion:%d, %d left", s.Id.Id, len(proxySessionMap))
	}
}
func sessionExist(sid SessionId) bool {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	_, exist := proxySessionMap[sid]
	return exist
}

func removeUserSessions(user string, runid int64) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	for k, s := range proxySessionMap {
		if k.User == user && k.RunId == runid {
			destroyProxySession(s)
		}
	}
}

func (p *ProxySession) publish(ev event.Event) {
	ev.SetId(p.Id.Id)
	start := time.Now()
	for {
		queue, match := getEventQueue(p.Id.ConnId, false)
		if !match {
			p.forceClose()
			return
		}
		if nil != queue {
			err := queue.Publish(ev, 5*time.Second)
			if nil != err {
				log.Printf("Session[%s:%d] write event error:%v.", p.Id.User, p.Id.Id, err)
				p.forceClose()
			}
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
		//log.Printf("Session[%s:%d] close connection to %s", p.Id.User, p.Id.Id, p.addr)
		c.Close()
		p.conn = nil
		p.addr = ""
	}
	return nil
}

func (p *ProxySession) forceClose() {
	p.close()
	removeProxySession(p)
}

func (p *ProxySession) initialClose() {
	if p.network == "tcp" {
		ev := &event.TCPCloseEvent{}
		p.publish(ev)
	}
	p.forceClose()
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

func (p *ProxySession) open(network, to string) error {
	if p.conn != nil && to == p.addr {
		return nil
	}
	p.close()
	p.network = network
	//log.Printf("Session[%s:%d] open connection to %s.", p.Id.User, p.Id.Id, to)
	c, err := net.DialTimeout(network, to, 5*time.Second)
	if nil != err {
		if network == "tcp" {
			ev := &event.TCPCloseEvent{}
			p.publish(ev)
		}
		log.Printf("Failed to connect %s for reason:%v", to, err)
		return err
	}
	p.conn = c
	p.addr = to
	go p.readNetwork()
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

func (p *ProxySession) readNetwork() error {
	for {
		conn := p.conn
		if nil == conn {
			return nil
		}
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		b := make([]byte, 8192)
		n, err := conn.Read(b)
		if n > 0 {
			var ev event.Event
			if p.network == "tcp" {
				ev = &event.TCPChunkEvent{Content: b[0:n]}
			} else {
				ev = &event.UDPEvent{Content: b[0:n]}
			}
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
	case *event.UDPEvent:
		err := p.open("udp", ev.(*event.UDPEvent).Addr)
		if nil != err {
			return err
		}
		_, err = p.write(ev.(*event.UDPEvent).Content)
		return err
	case *event.TCPOpenEvent:
		return p.open("tcp", ev.(*event.TCPOpenEvent).Addr)
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
		//log.Printf("Session[%d] %s %s", ev.GetId(), req.Method, req.URL)
		err := p.open("tcp", addr)
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

func authConnection(auth *event.AuthEvent, ctx *ConnContex) error {
	//log.Printf("###Recv auth IV = %d, ctx IV = %d", auth.IV, ctx.IV)
	if len(ctx.User) == 0 {
		if !ServerConf.VerifyUser(auth.User) {
			return fmt.Errorf("Auth failed with user:%s", auth.User)
		}
		authedUser := auth.User
		authedUser = authedUser + "@" + auth.Mac
		ctx.User = authedUser
		ctx.ConnIndex = int(auth.Index)
		ctx.IV = auth.IV
		ctx.RunId = auth.RunId
		cid := ctx.ConnId
		GetEventQueue(ctx.ConnId, true)
		//log.Printf("###Recv IV = %d", ctx.IV)
		lastRunId, ok := closeUnmatchedUserEventQueue(cid)
		if ok {
			log.Printf("Remove old user sessions for runid:%d, while new runid:%d", lastRunId, ctx.RunId)
			removeUserSessions(cid.User, lastRunId)
			closeUnmatchedUserEventQueue(cid)
		}
		return nil
	} else {
		return fmt.Errorf("Duplicate auth/login event in same connection")
	}
}

func handleEvent(ev event.Event, ctx *ConnContex) (event.Event, error) {
	switch ev.(type) {
	case *event.AuthEvent:
		auth := ev.(*event.AuthEvent)
		err := authConnection(auth, ctx)
		var authres event.NotifyEvent
		authres.SetId(ev.GetId())
		if nil == err {
			authres.Code = event.SuccessAuthed
		} else {
			authres.Code = event.ErrAuthFailed
		}
		return &authres, nil
	default:
		session := getProxySessionByEvent(ctx.ConnId, ev)
		if nil != session {
			session.offer(ev)
		} else {
			if _, ok := ev.(*event.TCPCloseEvent); !ok {
				log.Printf("No session:%d found for event %T", ev.GetId(), ev)
			}
		}
	}
	return nil, nil
}

func HandleRequestBuffer(reqbuf *bytes.Buffer, ctx *ConnContex) ([]event.Event, error) {
	var ress []event.Event
	for reqbuf.Len() > 0 {
		var ev event.Event
		var err error
		err, ev = event.DecryptEvent(reqbuf, ctx.IV)
		if nil != err {
			if err != event.EBNR {
				log.Printf("Failed to decode event for reason:%v", err)
			}
			return ress, err
		}
		res, err := handleEvent(ev, ctx)
		if nil != res {
			ress = append(ress, res)
		}
		if nil != err {
			return ress, err
		}
	}
	return ress, nil
}

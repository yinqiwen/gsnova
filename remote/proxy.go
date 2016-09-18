package remote

import (
	//"fmt"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
)

var proxySessionMap map[SessionId]*ProxySession = make(map[SessionId]*ProxySession)
var sessionSize int32
var sessionMutex sync.Mutex

type ConnId struct {
	User      string
	ConnIndex int
	RunId     int64
}

type ConnContext struct {
	event.CryptoContext
	ConnId
	//IV            uint64
	EncryptMethod int
	Closing       bool
}

func NewConnContext() *ConnContext {
	ctx := new(ConnContext)
	return ctx
}

type SessionId struct {
	ConnId
	Id uint32
}

type ProxySession struct {
	Id            SessionId
	CreateTime    time.Time
	conn          net.Conn
	addr          string
	network       string
	ch            chan event.Event
	closeByClient bool

	closed bool
}

func GetSessionTableSize() int {
	// sessionMutex.Lock()
	// defer sessionMutex.Unlock()
	return int(sessionSize)
}
func DumpAllSession(wr io.Writer) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	for _, session := range proxySessionMap {
		session.dump(wr)
	}
}

func getProxySessionByEvent(ctx *ConnContext, ev event.Event) *ProxySession {
	cid := ctx.ConnId
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
	atomic.AddInt32(&sessionSize, 1)
	return p
}

func destroyProxySession(s *ProxySession) {
	delete(proxySessionMap, s.Id)
	s.ch <- nil
	close(s.ch)
	s.closed = true
	atomic.AddInt32(&sessionSize, -1)
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

func removeUserSessions(user string, runid int64) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	for k, s := range proxySessionMap {
		if k.User == user && k.RunId == runid {
			destroyProxySession(s)
		}
	}
}

//just for debug
func (p *ProxySession) dump(wr io.Writer) {
	fmt.Fprintf(wr, "[%d]network=%s,addr=%s,closed=%v\n", p.Id.Id, p.network, p.addr, p.conn == nil)
}

func (p *ProxySession) publish(ev event.Event) {
	ev.SetId(p.Id.Id)
	start := time.Now()
	timeout := start.Add(60 * time.Second)
	for !p.closeByClient && time.Now().Before(timeout) {
		queue := getEventQueue(p.Id.ConnId, false)
		if nil != queue {
			err := queue.Publish(ev, 10*time.Second)
			if nil != err {
				continue
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !p.closeByClient {
		log.Printf("Session[%s:%d] publish event timeout after %v", p.Id.User, p.Id.Id, time.Now().Sub(start))
		p.forceClose()
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
		ev := &event.ConnCloseEvent{}
		p.publish(ev)
	}
	p.forceClose()
}

func (p *ProxySession) processEvents() {
	for {
		select {
		case ev := <-p.ch:
			if nil != ev {
				p.handle(ev)
			} else {
				return
			}
		case <-time.After(time.Second * 30):
			if nil == p.conn {
				return
			}
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
		p.initialClose()
		log.Printf("Failed to connect %s:%s for reason:%v", network, to, err)
		return err
	}
	p.conn = c
	p.addr = to
	go p.readNetwork()
	return nil
}

func (p *ProxySession) write(b []byte) (int, error) {
	if p.conn == nil {
		//log.Printf("Session[%s:%d] have no established connection to %s.", p.Id.User, p.Id.Id, p.addr)
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
	b := make([]byte, 8192)
	for {
		conn := p.conn
		if nil == conn {
			return nil
		}
		readTimeout := 0
		if p.network == "udp" {
			readTimeout = 30
		}
		if readTimeout > 0 {
			conn.SetReadDeadline(time.Now().Add(time.Duration(readTimeout) * time.Second))
		}
		n, err := conn.Read(b)
		if n > 0 {
			content := make([]byte, n)
			copy(content, b[0:n])
			var ev event.Event
			if p.network == "tcp" {
				ev = &event.TCPChunkEvent{Content: content}
			} else {
				ev = &event.UDPEvent{Content: content}
			}
			p.publish(ev)
		}
		if nil != err {
			break
		}

	}
	p.initialClose()
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
	case *event.ConnCloseEvent:
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

func authConnection(auth *event.AuthEvent, ctx *ConnContext) error {
	//log.Printf("###Recv auth IV = %d, ctx IV = %d", auth.IV, ctx.IV)
	if len(ctx.User) == 0 {
		if !ServerConf.VerifyUser(auth.User) {
			return fmt.Errorf("Auth failed with user:%s", auth.User)
		}
		authedUser := auth.User
		//authedUser = authedUser + "@" + auth.Mac
		ctx.User = authedUser
		ctx.ConnIndex = int(auth.Index)
		//ctx.IV = auth.IV
		ctx.RunId = auth.RunId
		ctx.CryptoContext.DecryptIV = auth.IV
		ctx.CryptoContext.EncryptIV = auth.IV
		ctx.CryptoContext.Method = auth.EncryptMethod
		GetEventQueue(ctx.ConnId, true)
		//log.Printf("###Recv IV = %d", ctx.IV)
		return nil
	} else {
		return fmt.Errorf("Duplicate auth/login event in same connection")
	}
}

func handleEvent(ev event.Event, ctx *ConnContext) (event.Event, error) {
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
	case *event.HeartBeatEvent:
		//do nothing
	case *event.ChannelCloseReqEvent:
		ctx.Closing = true
		queue := getEventQueue(ctx.ConnId, false)
		if nil != queue {
			queue.Publish(nil, 1*time.Minute)
		}
	case *event.ConnTestEvent:
		session := getProxySessionByEvent(ctx, ev)
		if nil == session {
			log.Printf("Session:%d is NOT exist now.", ev.GetId())
			queue := getEventQueue(ctx.ConnId, false)
			if nil != queue {
				closeEv := &event.ConnCloseEvent{}
				closeEv.SetId(ev.GetId())
				queue.Publish(closeEv, 10*time.Millisecond)
			}
		}
	default:
		session := getProxySessionByEvent(ctx, ev)
		if nil != session {
			session.offer(ev)
		}
		if _, ok := ev.(*event.ConnCloseEvent); !ok {
			if nil == session {
				log.Printf("No session:%d found for event %T", ev.GetId(), ev)
			}
		} else {
			if nil != session {
				session.closeByClient = true
			}
		}
	}
	return nil, nil
}

func HandleRequestBuffer(reqbuf *bytes.Buffer, ctx *ConnContext) ([]event.Event, error) {
	var ress []event.Event
	for reqbuf.Len() > 0 {
		var ev event.Event
		var err error

		err, ev = event.DecryptEvent(reqbuf, &ctx.CryptoContext)
		if nil != err {
			if err != event.EBNR {
				log.Printf("Failed to decode event for reason:%v  %d", err, ctx.CryptoContext.DecryptIV)
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

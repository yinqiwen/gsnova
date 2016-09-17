package proxy

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/google/btree"
	"github.com/yinqiwen/gsnova/common/event"
)

const (
	addrTypeIPv4  = 1
	addrTypeIPv6  = 6
	flagKeepAlive = uint8(1 << 0)
	flagReBind    = uint8(1 << 1)
	flagDNS       = uint8(1 << 2)
	flagIPv6      = uint8(1 << 3)
)

type udpSessionId struct {
	id         uint16
	activeTime time.Time
}

func (s *udpSessionId) Less(than btree.Item) bool {
	other := than.(*udpSessionId)
	if !s.activeTime.Equal(other.activeTime) {
		return s.activeTime.Before(other.activeTime)
	}
	return s.id < other.id
}

type udpSession struct {
	udpSessionId
	addr       udpgwAddr
	targetAddr string
	session    *ProxySession
}

type udpgwAddr struct {
	ip   net.IP
	port uint16
}

type udpgwPacket struct {
	length  uint16
	flags   uint8
	conid   uint16
	addr    udpgwAddr
	content []byte
}

func (u *udpgwPacket) address() string {
	if len(u.addr.ip) == 16 {
		u.addr.ip = u.addr.ip.To16()
	} else {
		u.addr.ip = u.addr.ip.To4()
	}
	return fmt.Sprintf("%s:%d", u.addr.ip.String(), u.addr.port)
}

func (u *udpgwPacket) write(w io.Writer) error {
	var buf bytes.Buffer
	u.length = 1 + 2 + uint16(len(u.addr.ip)) + 2 + uint16(len(u.content))
	binary.Write(&buf, binary.LittleEndian, u.length)
	binary.Write(&buf, binary.BigEndian, u.flags)
	binary.Write(&buf, binary.BigEndian, u.conid)
	buf.Write(u.addr.ip)
	binary.Write(&buf, binary.BigEndian, u.addr.port)
	buf.Write(u.content)
	_, err := w.Write(buf.Bytes())
	return err
}

func (u *udpgwPacket) read(r *bufio.Reader) error {
	bs, err := r.Peek(2)
	if nil != err {
		return err
	}
	u.length = binary.LittleEndian.Uint16(bs)
	//binary.Read(r, binary.BigEndian, &u.length)
	r.Discard(2)
	//log.Printf("###First %d  %d %d %p", u.length, binary.BigEndian.Uint16(bs), len(bs), r)
	_, err = r.Peek(int(u.length))
	if nil != err {
		//log.Printf("### %v", err)
		return err
	}
	bodylen := u.length
	binary.Read(r, binary.BigEndian, &u.flags)
	binary.Read(r, binary.BigEndian, &u.conid)
	bodylen -= 3
	if bodylen > 0 {
		if (u.flags & flagIPv6) != 0 {
			u.addr.ip = make(net.IP, 16)

		} else {
			u.addr.ip = make(net.IP, 4)
		}
		r.Read(u.addr.ip)
		bodylen -= uint16(len(u.addr.ip))
		binary.Read(r, binary.BigEndian, &u.addr.port)
		bodylen -= 2
		u.content = make([]byte, int(bodylen))
		r.Read(u.content)
	}
	return nil
}

var udpSessionTable = make(map[uint16]*udpSession)
var udpSessionIdSet = btree.New(4)
var cidTable = make(map[uint32]uint16)
var udpSessionMutex sync.Mutex

func closeAllUDPSession() {
	udpSessionMutex.Lock()
	defer udpSessionMutex.Unlock()
	for id, _ := range udpSessionTable {
		delete(udpSessionTable, id)
		//closeProxySession(session.session.id)
	}
	cidTable = make(map[uint32]uint16)
}

func removeUdpSession(id *udpSessionId) {
	s, exist := udpSessionTable[id.id]
	if exist {
		log.Printf("Delete %d(%d) udpsession", id.id, s.session.id)
		delete(cidTable, s.session.id)
		delete(udpSessionTable, s.id)
		closeProxySession(s.session.id)
	}
}

func init() {
	go expireUdpSessions()
}

func expireUdpSessions() {
	ticker := time.NewTicker(1 * time.Second)
	removeExpiredSession := func() {
		udpSessionMutex.Lock()
		defer udpSessionMutex.Unlock()
		for i := 0; i < 5; i++ {
			tmp := udpSessionIdSet.Min()
			if nil != tmp {
				id := tmp.(*udpSessionId)
				if id.activeTime.Add(20 * time.Second).Before(time.Now()) {
					removeUdpSession(id)
				} else {
					return
				}
			}
		}
	}
	for {
		select {
		case <-ticker.C:
			removeExpiredSession()
		}
	}
}

func updateUdpSession(u *udpSession) {
	udpSessionMutex.Lock()
	defer udpSessionMutex.Unlock()
	if u.activeTime.Unix() != 0 {
		udpSessionIdSet.Delete(&u.udpSessionId)
	}
	u.activeTime = time.Now()
	udpSessionIdSet.ReplaceOrInsert(&u.udpSessionId)
}

func getUDPSession(id uint16, queue *event.EventQueue, createIfMissing bool) *udpSession {
	udpSessionMutex.Lock()
	defer udpSessionMutex.Unlock()
	session, exist := udpSessionTable[id]
	if !exist {
		if createIfMissing {
			s := new(udpSession)
			s.session = newProxySession(getSessionId(), queue)
			s.id = id
			udpSessionTable[id] = s
			cidTable[s.session.id] = id
			return s
		}
		return nil
	}
	return session
}
func getCid(sid uint32) (uint16, bool) {
	udpSessionMutex.Lock()
	defer udpSessionMutex.Unlock()
	cid, exist := cidTable[sid]
	return cid, exist
}

func handleUDPGatewayConn(conn net.Conn, proxy ProxyConfig) {
	queue := event.NewEventQueue()
	connClosed := false
	go func() {
		for !connClosed {
			ev, err := queue.Read(1 * time.Second)
			if err != nil {
				if err != io.EOF {
					continue
				}
				return
			}
			switch ev.(type) {
			case *event.UDPEvent:
				cid, exist := getCid(ev.GetId())
				if exist {
					usession := getUDPSession(cid, nil, false)
					if nil != usession {
						var packet udpgwPacket
						packet.content = ev.(*event.UDPEvent).Content
						packet.addr = usession.addr
						packet.conid = cid
						if len(usession.addr.ip) == 16 {
							packet.flags = flagIPv6
						}
						err = packet.write(conn)
						if nil != err {
							//log.Printf("###write udp error %v", err)
							connClosed = true
							conn.Close()
						} else {
							//log.Printf("###UDP Recv for %d(%d)", usession.session.id, usession.id)
							updateUdpSession(usession)
						}
						continue
					}
				}
				log.Printf("No udp session found for %d", ev.GetId())
			default:
				log.Printf("Invalid event type:%T to process", ev)
			}
		}
	}()

	bufconn := bufio.NewReader(conn)
	for {
		var packet udpgwPacket
		err := packet.read(bufconn)
		if nil != err {
			if err == event.EBNR {
				continue
			} else {
				log.Printf("Failed to read udpgw packet:%v", err)
				conn.Close()
				connClosed = true
				return
			}
		}
		if len(packet.content) == 0 {
			continue
			//log.Printf("###Recv udpgw packet to %s:%d", packet.addr.ip.String(), packet.addr.port)
		}

		usession := getUDPSession(packet.conid, queue, true)
		usession.addr = packet.addr
		updateUdpSession(usession)
		usession.activeTime = time.Now()

		ev := &event.UDPEvent{Content: packet.content, Addr: packet.address()}
		ev.SetId(usession.session.id)
		var p Proxy
		if packet.addr.port == 53 {
			p = proxy.findProxyByRequest("dns", packet.addr.ip.String(), nil)
			if p.Config().IsDirect() {
				go func() {
					res, err := dnsQueryRaw(packet.content)
					if nil == err {
						resev := &event.UDPEvent{}
						resev.Content = res
						resev.SetId(usession.session.id)
						HandleEvent(resev)
					} else {
						log.Printf("[ERROR]Failed to query dns with reason:%v", err)
					}
				}()
				continue
				//ev.Addr = selectDNSServer()
				//dnsQueryRaw(packet.content)
			}
		} else {
			//log.Printf("###Recv non dns udp to %s:%d", packet.addr.ip.String(), packet.addr.port)
			p = proxy.findProxyByRequest("udp", packet.addr.ip.String(), nil)
		}
		if len(usession.targetAddr) > 0 {
			if usession.targetAddr != ev.Addr {
				closeEv := &event.ConnCloseEvent{}
				closeEv.SetId(usession.session.id)
				p.Serve(usession.session, closeEv)
			}
		}
		usession.targetAddr = ev.Addr

		if nil != p {
			//log.Printf("Session:%d(%d) request udp:%s", usession.session.id, usession.id, ev.Addr)
			p.Serve(usession.session, ev)
		}
	}
}

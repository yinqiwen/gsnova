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

	"github.com/golang/snappy"
	"github.com/google/btree"
	"github.com/yinqiwen/gsnova/common/mux"
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
	localConn  net.Conn
}

func (u *udpSession) Write(content []byte) error {
	var packet udpgwPacket
	packet.content = content
	packet.addr = u.addr
	packet.conid = u.udpSessionId.id
	if len(u.addr.ip) == 16 {
		packet.flags = flagIPv6
	}
	return packet.write(u.localConn)

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
		log.Printf("Delete %d udpsession", id.id)
		delete(udpSessionTable, s.id)
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

func getUDPSession(id uint16, conn net.Conn, createIfMissing bool) *udpSession {
	udpSessionMutex.Lock()
	defer udpSessionMutex.Unlock()
	usession, exist := udpSessionTable[id]
	if !exist {
		if createIfMissing {
			s := new(udpSession)
			s.localConn = conn
			s.id = id
			udpSessionTable[id] = s
			//cidTable[s.session.id] = id
			return s
		}
		return nil
	}
	return usession
}
func getCid(sid uint32) (uint16, bool) {
	udpSessionMutex.Lock()
	defer udpSessionMutex.Unlock()
	cid, exist := cidTable[sid]
	return cid, exist
}

func handleUDPGatewayConn(localConn net.Conn, proxy ProxyConfig) {
	var proxyChannelName string
	bufconn := bufio.NewReader(localConn)
	var stream mux.MuxStream
	var streamReader io.Reader
	var streamWriter io.Writer
	defer func() {
		localConn.Close()
		if nil != stream {
			stream.Close()
		}
	}()
	for {
		var packet udpgwPacket
		err := packet.read(bufconn)
		if nil != err {
			log.Printf("Failed to read udpgw packet:%v", err)
			localConn.Close()
			return
		}
		if len(packet.content) == 0 {
			continue
			//log.Printf("###Recv udpgw packet to %s:%d", packet.addr.ip.String(), packet.addr.port)
		}

		usession := getUDPSession(packet.conid, localConn, true)
		usession.addr = packet.addr
		updateUdpSession(usession)

		if packet.addr.port == 53 {
			selectProxy := proxy.findProxyChannelByRequest("dns", packet.addr.ip.String(), nil)
			if selectProxy == directProxyChannelName {
				go func() {
					res, err := dnsQueryRaw(packet.content)
					if nil == err {
						err = usession.Write(res)
					}
					if nil != err {
						log.Printf("[ERROR]Failed to query dns with reason:%v", err)
						return
					}
				}()
				continue
			}
			proxyChannelName = selectProxy
		}

		if len(usession.targetAddr) > 0 {
			if usession.targetAddr != packet.address() {
				if nil != stream {
					stream.Close()
					stream = nil
				}
			}
		}

		if nil == stream {
			proxyChannelName = proxy.findProxyChannelByRequest("udp", packet.addr.ip.String(), nil)
			if len(proxyChannelName) == 0 {
				log.Printf("[ERROR]No proxy found for udp to %s", packet.addr.ip.String())
				return
			}
			stream, conf, err := getMuxStreamByChannel(proxyChannelName)
			if nil != err {
				log.Printf("[ERROR]Failed to create mux stream:%v", err)
				return
			}
			stream.Connect("udp", packet.address())
			streamReader = stream
			streamWriter = stream
			if conf.Compressor == mux.SnappyCompressor {
				streamReader = snappy.NewReader(stream)
				streamWriter = snappy.NewWriter(stream)
			}
			go func() {
				b := make([]byte, 8192)
				for {
					n, err := streamReader.Read(b)
					if n > 0 {
						err = usession.Write(b[0:n])
					}
					if nil != err {
						stream.Close()
						return
					}
				}
			}()
		} else {
			streamWriter.Write(packet.content)
		}
	}
}

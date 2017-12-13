package local

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/google/btree"
	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/dns"
	"github.com/yinqiwen/gsnova/common/logger"
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

type udpSession struct {
	udpSessionId
	addr             udpgwAddr
	targetAddr       string
	localConn        net.Conn
	stream           mux.MuxStream
	streamWriter     io.Writer
	streamReader     io.Reader
	proxyChannelName string
}

func (u *udpSession) closeStream() {
	if nil != u.stream {
		u.stream.Close()
		u.stream = nil
		u.streamWriter = nil
		u.streamReader = nil
	}
}
func (u *udpSession) close() {
	u.closeStream()
	removeUdpSession(&u.udpSessionId, 0)
	updateUdpSession(u, true)
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

func (u *udpSession) handlePacket(proxy *ProxyConfig, packet *udpgwPacket) error {
	if nil != u.streamWriter {
		u.streamWriter.Write(packet.content)
		return nil
	}

	remoteAddr := packet.address()
	if packet.addr.port == 53 {
		selectProxy := proxy.findProxyChannelByRequest("dns", packet.addr.ip.String(), nil)
		if selectProxy == channel.DirectChannelName {
			res, err := dns.LocalDNS.QueryRaw(packet.content)
			if nil == err {
				err = u.Write(res)
			}
			if nil != err {
				logger.Error("[ERROR]Failed to query dns with reason:%v", err)
			}
			u.close()
			return err
		}
		u.proxyChannelName = selectProxy
		if len(GConf.LocalDNS.TrustedDNS) > 0 {
			remoteAddr = GConf.LocalDNS.TrustedDNS[0]
		}
	}
	if len(u.proxyChannelName) == 0 {
		u.proxyChannelName = proxy.findProxyChannelByRequest("udp", packet.addr.ip.String(), nil)
	}
	if len(u.proxyChannelName) == 0 {
		logger.Error("[ERROR]No proxy found for udp to %s", packet.addr.ip.String())
		return nil
	}

	if len(u.targetAddr) > 0 {
		if u.targetAddr != packet.address() {
			u.closeStream()
		}
	}
	stream, conf, err := channel.GetMuxStreamByChannel(u.proxyChannelName)
	readTimeoutMS := conf.RemoteUDPReadMSTimeout
	if packet.addr.port == 53 {
		readTimeoutMS = conf.RemoteDNSReadMSTimeout
	}
	if nil != stream {
		opt := mux.StreamOptions{
			DialTimeout: conf.RemoteDialMSTimeout,
			ReadTimeout: readTimeoutMS,
		}
		err = stream.Connect("udp", remoteAddr, opt)
	}
	if nil != err {
		logger.Error("[ERROR]Failed to create mux stream:%v for proxy:%s by address:%v", err, u.proxyChannelName, packet.addr)
		return err
	}

	u.stream = stream
	u.streamReader, u.streamWriter = mux.GetCompressStreamReaderWriter(stream, conf.Compressor)
	go func() {
		b := make([]byte, 8192)
		for {
			stream.SetReadDeadline(time.Now().Add(time.Duration(readTimeoutMS) * time.Millisecond))
			n, err := u.streamReader.Read(b)
			if n > 0 {
				err = u.Write(b[0:n])
			}
			if nil != err {
				break
			}
		}

	}()
	u.streamWriter.Write(packet.content)
	return nil
}

var udpSessionTable sync.Map
var udpSessionIdSet = btree.New(4)
var cidTable = make(map[uint32]uint16)
var udpSessionMutex sync.Mutex

func closeAllUDPSession() {
	udpSessionTable.Range(func(key, value interface{}) bool {
		session := value.(*udpSession)
		session.closeStream()
		return true
	})
	udpSessionTable = sync.Map{}
	cidTable = make(map[uint32]uint16)
}

func removeUdpSession(id *udpSessionId, expireTime time.Duration) {
	v, exist := udpSessionTable.Load(id.id)
	if exist {
		if expireTime > 0 {
			logger.Debug("Delete udpsession:%d since it's not active since %v ago.", id.id, expireTime)
		}
		v.(*udpSession).closeStream()
		udpSessionTable.Delete(id.id)
		//delete(udpSessionTable, s.id)
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
				expireTime := time.Now().Sub(id.activeTime)
				if expireTime >= 30*time.Second {
					udpSessionIdSet.Delete(id)
					removeUdpSession(id, expireTime)
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

func updateUdpSession(u *udpSession, remove bool) {
	udpSessionMutex.Lock()
	defer udpSessionMutex.Unlock()
	if !u.activeTime.IsZero() {
		udpSessionIdSet.Delete(&u.udpSessionId)
	}
	if !remove {
		u.activeTime = time.Now()
		udpSessionIdSet.ReplaceOrInsert(&u.udpSessionId)
	}
}

func getUDPSession(id uint16, conn net.Conn, createIfMissing bool) *udpSession {
	udpSessionMutex.Lock()
	defer udpSessionMutex.Unlock()
	usession, exist := udpSessionTable.Load(id)
	if !exist {
		if createIfMissing {
			s := new(udpSession)
			s.localConn = conn
			s.id = id
			udpSessionTable.Store(id, s)
			//cidTable[s.session.id] = id
			return s
		}
		return nil
	}
	return usession.(*udpSession)
}
func getCid(sid uint32) (uint16, bool) {
	udpSessionMutex.Lock()
	defer udpSessionMutex.Unlock()
	cid, exist := cidTable[sid]
	return cid, exist
}

func handleUDPGatewayConn(localConn net.Conn, proxy *ProxyConfig) {
	bufconn := bufio.NewReader(localConn)
	defer func() {
		localConn.Close()
	}()
	for {
		var packet udpgwPacket
		err := packet.read(bufconn)
		if nil != err {
			logger.Error("Failed to read udpgw packet:%v", err)
			localConn.Close()
			return
		}
		if len(packet.content) == 0 {
			continue
			//log.Printf("###Recv udpgw packet to %s:%d", packet.addr.ip.String(), packet.addr.port)
		}

		usession := getUDPSession(packet.conid, localConn, true)
		usession.addr = packet.addr
		updateUdpSession(usession, false)
		go usession.handlePacket(proxy, &packet)
	}
}

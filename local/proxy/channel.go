package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
)

const (
	stateCloseToSendReq  = 1
	stateCloseWaitingACK = 2
)

var ErrChannelReadTimeout = errors.New("Remote channel read timeout")
var ErrChannelAuthFailed = errors.New("Remote channel auth failed")

type ProxyChannel interface {
	Write(event.Event) (event.Event, error)
}

type RemoteProxyChannel interface {
	Open() error
	SetCryptoCtx(ctx *event.CryptoContext)
	Closed() bool
	Request([]byte) ([]byte, error)
	ReadTimeout() time.Duration
	HandleCtrlEvent(ev event.Event)
	io.ReadWriteCloser
}

type RemoteChannel struct {
	Addr                string
	Index               int
	DirectIO            bool
	WriteJoinAuth       bool
	OpenJoinAuth        bool
	SecureTransport     bool
	HeartBeatPeriod     int
	ReconnectPeriod     int
	RCPRandomAdjustment int
	C                   RemoteProxyChannel

	connSendedEvents uint32
	authResult       int
	cryptoCtx        event.CryptoContext
	//iv               uint64
	wch     chan event.Event
	running bool

	connectTime       time.Time
	nextReconnectTime time.Time
	closeState        int

	activeSessionNum int32
}

func (rc *RemoteChannel) updateActiveSessionNum(delta int32) {
	atomic.AddInt32(&rc.activeSessionNum, delta)
}
func (rc *RemoteChannel) GetActiveSessionNum() int32 {
	return rc.activeSessionNum
}

func (rc *RemoteChannel) authed() bool {
	return rc.authResult != 0
}

func randCryptoCtx() event.CryptoContext {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	tmp := uint64(r.Int63())
	var ctx event.CryptoContext
	ctx.EncryptIV = tmp
	ctx.DecryptIV = tmp
	ctx.Method = event.GetDefaultCryptoMethod()
	return ctx
}
func (rc *RemoteChannel) resetCryptoCtx() {
	rc.cryptoCtx = randCryptoCtx()
	if rc.SecureTransport && strings.EqualFold(GConf.Encrypt.Method, "auto") {
		rc.cryptoCtx.Method = 0
	}
	//log.Printf("Channel[%d] reset IV:%d.", rc.Index, rc.cryptoCtx.EncryptIV)
}

func (rc *RemoteChannel) Init(authRequired bool) error {
	rc.running = true
	if !rc.DirectIO {
		rc.wch = make(chan event.Event, 5)
		go rc.processWrite()
		go rc.processRead()
	}
	if rc.HeartBeatPeriod > 0 {
		go rc.heartbeat()
	}
	if !authRequired {
		rc.authResult = event.SuccessAuthed
		return nil
	}
	rc.authResult = 0
	start := time.Now()
	authTimeout := rc.C.ReadTimeout()
	for rc.authResult == 0 {
		if time.Now().After(start.Add(authTimeout)) {
			rc.Stop()
			rc.authResult = -1 //timeout
			return fmt.Errorf("Server:%s auth timeout after %v", rc.Addr, time.Now().Sub(start))
		}
		time.Sleep(1 * time.Millisecond)
	}
	if rc.authResult == event.ErrAuthFailed {
		rc.Stop()
		return fmt.Errorf("Server:%s auth failed.", rc.Addr)
	} else if rc.authResult == event.SuccessAuthed {
		log.Printf("Server:%s authed success.", rc.Addr)
	} else {
		return fmt.Errorf("Server:%s auth recv unexpected code:%d.", rc.Addr, rc.authResult)
	}
	//closeProxySession(authSession.id)
	return nil
}
func (rc *RemoteChannel) Close() {
	c := rc.C
	if nil != c {
		c.Close()
	}
}
func (rc *RemoteChannel) Stop() {
	rc.running = false
	rc.Close()
}

func (rc *RemoteChannel) heartbeat() {
	ticker := time.NewTicker(time.Duration(rc.HeartBeatPeriod) * time.Second)
	for rc.running {
		select {
		case <-ticker.C:
			if !rc.C.Closed() && (GConf.ChannelKeepAlive || getProxySessionSize() > 0) {
				rc.Write(event.NewHeartBeatEvent())
			}
		}
	}
}

func (rc *RemoteChannel) processWrite() {
	readBufferEv := func(evs []event.Event) []event.Event {
		sev := <-rc.wch
		if nil != sev {
			evs = append(evs, sev)
		}
		return evs
	}
	var sendEvents []event.Event
	var wbuf bytes.Buffer
	for rc.running {
		conn := rc.C
		//disable write if waiting for close CK
		if rc.closeState == stateCloseWaitingACK {
			time.Sleep(1 * time.Millisecond)
			continue
		}

		if len(sendEvents) == 0 {
			if len(rc.wch) > 0 {
				for len(rc.wch) > 0 {
					sendEvents = readBufferEv(sendEvents)
				}
			} else {
				sendEvents = readBufferEv(sendEvents)
			}
		}

		if !rc.running && len(sendEvents) == 0 {
			return
		}
		if conn.Closed() {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		wbuf.Reset()
		//cryptoCtx := rc.cryptoCtx
		if rc.WriteJoinAuth || (rc.connSendedEvents == 0 && rc.OpenJoinAuth) {
			auth := NewAuthEvent(rc.SecureTransport)
			auth.Index = int64(rc.Index)
			auth.IV = rc.cryptoCtx.EncryptIV
			event.EncryptEvent(&wbuf, auth, &rc.cryptoCtx)
			rc.connSendedEvents++
		}

		for _, sev := range sendEvents {
			if auth, ok := sev.(*event.AuthEvent); ok {
				if auth.IV != rc.cryptoCtx.EncryptIV {
					log.Printf("####Got %d %d", rc.cryptoCtx.EncryptIV, auth.IV)
				}
				auth.IV = rc.cryptoCtx.EncryptIV
				//log.Printf("##[%d]Send %T with %d", sev.GetId(), sev, 0)
			}
			event.EncryptEvent(&wbuf, sev, &rc.cryptoCtx)
		}
		if rc.closeState == stateCloseToSendReq {
			closeReq := &event.ChannelCloseReqEvent{}
			event.EncryptEvent(&wbuf, closeReq, &rc.cryptoCtx)
		}
		rc.connSendedEvents += uint32(len(sendEvents))

		if wbuf.Len() > 0 {
			//start := time.Now()
			_, err := conn.Write(wbuf.Bytes())
			if nil != err {
				conn.Close()
				log.Printf("Failed to write tcp messgage:%v", err)
				//resend `sendEvents` in next process
			} else {
				//log.Printf("[%d]%s cost %v to write %d events.", rc.Index, rc.Addr, time.Now().Sub(start), len(sendEvents))
				sendEvents = nil
				//set state if write success
				if rc.closeState == stateCloseToSendReq {
					rc.closeState = stateCloseWaitingACK
				}
			}
		}
	}
}

func (rc *RemoteChannel) processRead() {
	var buf bytes.Buffer
	reconnectCount := 0
	for rc.running {
		conn := rc.C
		if conn.Closed() {
			rc.closeState = 0
			if rc.authed() && getProxySessionSize() == 0 && !GConf.ChannelKeepAlive {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if !rc.authed() && reconnectCount > 0 {
				rc.Stop()
				rc.authResult = event.ErrAuthFailed
				log.Printf("Channel[%d] auth failed since remote server disconnect.", rc.Index)
				return
			}
			rc.resetCryptoCtx()
			buf.Reset()
			rc.connSendedEvents = 0
			conn.SetCryptoCtx(&rc.cryptoCtx)
			err := conn.Open()
			reconnectCount++
			if nil != err {
				log.Printf("Channel[%d] connect %s failed:%v.", rc.Index, rc.Addr, err)
				time.Sleep(1 * time.Second)
				continue
			}
			rc.connectTime = time.Now()
			if rc.ReconnectPeriod > 0 {
				period := rc.ReconnectPeriod
				if rc.RCPRandomAdjustment > 0 && rc.RCPRandomAdjustment < period {
					period = helper.RandBetween(period-rc.RCPRandomAdjustment, period+rc.RCPRandomAdjustment)
				}
				rc.nextReconnectTime = rc.connectTime.Add(time.Duration(period) * time.Second)
			}
			log.Printf("Channel[%d] connect %s success.", rc.Index, rc.Addr)
			if rc.OpenJoinAuth {
				rc.Write(nil)
			}
		}
		reader := &helper.BufferChunkReader{conn, nil}
		for {
			//buf.Truncate(buf.Len())
			buf.Grow(8192)
			buf.ReadFrom(reader)
			cerr := reader.Err
			//n, cerr := conn.Read(data)
			//buf.Write(data[0:n])
			if rc.ReconnectPeriod > 0 && rc.closeState == 0 && nil == cerr {
				if rc.nextReconnectTime.Before(time.Now()) {
					rc.closeState = stateCloseToSendReq
					rc.Write(nil) //trigger to write ChannelCloseReqEvent
					log.Printf("Channel[%d] prepare to close %s to reconnect.", rc.Index, rc.Addr)
				}
			}
			for buf.Len() > 0 {
				err, ev := event.DecryptEvent(&buf, &rc.cryptoCtx)
				if nil != err {
					if err == event.EBNR {
						err = nil
					} else {
						log.Printf("Channel[%d]Failed to decode event for reason:%v with iv:%d", rc.Index, err, rc.cryptoCtx.DecryptIV)
						conn.Close()
					}
					break
				}
				switch ev.(type) {
				case *event.NotifyEvent:
					if !rc.authed() {
						auth := ev.(*event.NotifyEvent)
						rc.authResult = int(auth.Code)
						if rc.authResult != event.SuccessAuthed {
							rc.Stop()
							return
						}
						continue
					}
				case *event.ChannelCloseACKEvent:
					conn.Close()
					log.Printf("Channel[%d] close %s after recved close ACK.", rc.Index, rc.Addr)
					continue
				case *event.PortUnicastEvent:
					//log.Printf("Channel[%d] recv %v.", rc.Index, ev)
					rc.C.HandleCtrlEvent(ev)
					continue
				}
				if !rc.authed() {
					log.Printf("[ERROR]Expected auth result event for auth all connection, but got %T.", ev)
					conn.Close()
					continue
				}
				HandleEvent(ev)
			}
			if nil != cerr {
				if cerr != io.EOF && cerr != ErrChannelReadTimeout {
					log.Printf("Failed to read channel for reason:%v", cerr)
				}
				conn.Close()
				break
			}
		}
	}
}

func (rc *RemoteChannel) Request(ev event.Event) (event.Event, error) {
	var buf bytes.Buffer
	auth := NewAuthEvent(rc.SecureTransport)
	auth.Index = int64(rc.Index)
	if auth.EncryptMethod == event.Chacha20Encrypter {
		auth.EncryptMethod = event.Salsa20Encrypter
	}
	ctx := randCryptoCtx()
	ctx.Method = auth.EncryptMethod
	auth.IV = ctx.EncryptIV
	event.EncryptEvent(&buf, auth, &ctx)
	event.EncryptEvent(&buf, ev, &ctx)
	//event.EncodeEvent(&buf, ev)
	res, err := rc.C.Request(buf.Bytes())
	if nil != err {
		return nil, err
	}
	rbuf := bytes.NewBuffer(res)
	var rev event.Event
	err, rev = event.DecryptEvent(rbuf, &ctx)
	if nil != err {
		return nil, err
	}
	return rev, nil
}

func (rc *RemoteChannel) Write(ev event.Event) error {
	// if nil != ev {
	// 	rc.updateActiveSid(ev.GetId(), true)
	// }
	rc.wch <- ev
	return nil
}

func (rc *RemoteChannel) WriteRaw(p []byte) (int, error) {
	return rc.C.Write(p)
}

type RemoteChannelTable struct {
	cs []*RemoteChannel
	//cursor int
	mutex sync.Mutex
}

func (p *RemoteChannelTable) PrintStat(w io.Writer) {
	for _, c := range p.cs {
		fmt.Fprintf(w, "Channel[%s:%d]:SessionNum=%d\n", c.Addr, c.Index, c.GetActiveSessionNum())
	}
}

func (p *RemoteChannelTable) Add(c *RemoteChannel) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.cs = append(p.cs, c)
}

func (p *RemoteChannelTable) StopAll() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	for _, c := range p.cs {
		c.Stop()
	}
	p.cs = make([]*RemoteChannel, 0)
}

func (p *RemoteChannelTable) Select() *RemoteChannel {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// if len(p.cs) == 0 {
	// 	return nil
	// }
	var selected *RemoteChannel
	for _, r := range p.cs {
		if nil == selected || r.activeSessionNum < selected.activeSessionNum {
			selected = r
		}
	}
	// startCursor := p.cursor
	// for {
	// 	if p.cursor >= len(p.cs) {
	// 		p.cursor = 0
	// 	}
	// 	c := p.cs[p.cursor]
	// 	p.cursor++
	// 	if nil != c {
	// 		return c
	// 	}
	// 	if p.cursor == startCursor {
	// 		break
	// 	}
	// }
	return selected
}

func NewRemoteChannelTable() *RemoteChannelTable {
	p := new(RemoteChannelTable)
	p.cs = make([]*RemoteChannel, 0)
	return p
}

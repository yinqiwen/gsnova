package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
)

var ErrChannelReadTimeout = errors.New("Remote channel read timeout")

type ProxyChannel interface {
	Write(event.Event) (event.Event, error)
}

type RemoteProxyChannel interface {
	Open() error
	Closed() bool
	Request([]byte) ([]byte, error)
	io.ReadWriteCloser
}

type RemoteChannel struct {
	Addr          string
	Index         int
	DirectWrite   bool
	DirectRead    bool
	JoinAuthEvent bool
	C             RemoteProxyChannel

	authCode int
	wch      chan event.Event
	running  bool
}

func (rc *RemoteChannel) Init() error {
	rc.running = true
	rc.authCode = -1

	connAuthed := rc.C.Closed()
	if !rc.DirectWrite {
		rc.wch = make(chan event.Event, 5)
		go rc.processWrite()
	}
	if !rc.DirectRead {
		go rc.processRead()
	}

	if rc.Index < 0 {
		if !connAuthed {
			auth := NewAuthEvent()
			auth.Index = int64(rc.Index)
			rc.Write(auth)
		}
		start := time.Now()
		for rc.authCode != 0 {
			if time.Now().After(start.Add(5*time.Second)) || rc.authCode > 0 {
				rc.Stop()
				return fmt.Errorf("Server:%s auth failed", rc.Addr)
			}
			time.Sleep(1 * time.Millisecond)
		}
		log.Printf("Server:%s authed suucess.", rc.Addr)
	}
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

func (rc *RemoteChannel) processWrite() {
	auth := NewAuthEvent()
	auth.Index = int64(rc.Index)

	readBufferEv := func(buf *bytes.Buffer) int {
		sev := <-rc.wch
		if nil != sev {
			event.EncodeEvent(buf, sev)
			return 1
		}
		return 0
	}
	for rc.running {
		conn := rc.C
		if conn.Closed() {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		var buf bytes.Buffer
		if rc.JoinAuthEvent {
			event.EncodeEvent(&buf, auth)
		}
		count := 0
		if len(rc.wch) > 0 {
			for len(rc.wch) > 0 {
				count += readBufferEv(&buf)
			}
		} else {
			count += readBufferEv(&buf)
		}

		if !rc.running && count == 0 {
			return
		}
		if buf.Len() > 0 {
			start := time.Now()
			_, err := conn.Write(buf.Bytes())
			if nil != err {
				rc.Close()
				log.Printf("Failed to write tcp messgage:%v", err)
			} else {
				log.Printf("[%d]%s cost %v to write %d events.", rc.Index, rc.Addr, time.Now().Sub(start), count)
			}
		}
	}
}

func (rc *RemoteChannel) processRead() {
	for rc.running {
		conn := rc.C
		if conn.Closed() {
			err := conn.Open()
			if nil != err {
				log.Printf("Channel[%d] connect %s failed:%v.", rc.Index, rc.Addr, err)
				time.Sleep(1 * time.Second)
				continue
			}
			log.Printf("Channel[%d] connect %s success.", rc.Index, rc.Addr)
			auth := NewAuthEvent()
			auth.Index = int64(rc.Index)
			rc.Write(auth)
		}
		data := make([]byte, 8192)
		var buf bytes.Buffer
		for {
			n, cerr := conn.Read(data)
			buf.Write(data[0:n])
			for buf.Len() > 0 {
				err, ev := event.DecodeEvent(&buf)
				if nil != err {
					if err == event.EBNR {
						err = nil
					} else {
						log.Printf("Failed to decode event for reason:%v", err)
						conn.Close()
					}
					break
				}
				if rc.Index < 0 {
					if auth, ok := ev.(*event.ErrorEvent); ok {
						rc.authCode = int(auth.Code)
					} else {
						log.Printf("[ERROR]Expected error event for auth all connection, but got %T.", ev)
					}
					rc.Stop()
					return
				} else {
					HandleEvent(ev)
				}
			}
			if nil != cerr {
				if cerr != io.EOF && cerr != ErrChannelReadTimeout {
					log.Printf("Failed to read channel for reason:%v", cerr)
				}
				break
			}
		}
	}
}

func (rc *RemoteChannel) Request(ev event.Event) (event.Event, error) {
	var buf bytes.Buffer
	if rc.JoinAuthEvent {
		auth := NewAuthEvent()
		auth.Index = int64(rc.Index)
		event.EncodeEvent(&buf, auth)
	}
	event.EncodeEvent(&buf, ev)
	res, err := rc.C.Request(buf.Bytes())
	if nil != err {
		return nil, err
	}
	rbuf := bytes.NewBuffer(res)
	var rev event.Event
	err, rev = event.DecodeEvent(rbuf)
	if nil != err {
		return nil, err
	}
	return rev, nil
}

func (rc *RemoteChannel) Write(ev event.Event) error {
	if rc.DirectWrite {
		var buf bytes.Buffer
		if rc.JoinAuthEvent {
			auth := NewAuthEvent()
			auth.Index = int64(rc.Index)
			event.EncodeEvent(&buf, auth)
		}
		event.EncodeEvent(&buf, ev)
		_, err := rc.C.Write(buf.Bytes())
		return err
	} else {
		rc.wch <- ev
		return nil
	}
}

func (rc *RemoteChannel) WriteRaw(p []byte) (int, error) {
	return rc.C.Write(p)
}

type RemoteChannelTable struct {
	cs     []*RemoteChannel
	cursor int
	mutex  sync.Mutex
}

func (p *RemoteChannelTable) Add(c *RemoteChannel) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.cs = append(p.cs, c)
}

func (p *RemoteChannelTable) Select() *RemoteChannel {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(p.cs) == 0 {
		return nil
	}
	startCursor := p.cursor
	for {
		if p.cursor >= len(p.cs) {
			p.cursor = 0
		}
		c := p.cs[p.cursor]
		p.cursor++
		if nil != c {
			return c
		}
		if p.cursor == startCursor {
			break
		}
	}
	return nil
}

func NewRemoteChannelTable() *RemoteChannelTable {
	p := new(RemoteChannelTable)
	p.cs = make([]*RemoteChannel, 0)
	return p
}

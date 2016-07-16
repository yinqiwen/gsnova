package proxy

import (
	"sync"

	"github.com/yinqiwen/gsnova/common/event"
)

type ProxyChannel interface {
	Write(event.Event) (event.Event, error)
}

type ProxyChannelTable struct {
	cs     []ProxyChannel
	cursor int
	mutex  sync.Mutex
}

func (p *ProxyChannelTable) Add(c ProxyChannel) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.cs = append(p.cs, c)
}

func (p *ProxyChannelTable) Select() ProxyChannel {
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

func NewProxyChannelTable() *ProxyChannelTable {
	p := new(ProxyChannelTable)
	p.cs = make([]ProxyChannel, 0)
	return p
}

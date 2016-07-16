package paas

import (
	"fmt"
	"log"
	"strings"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type PaasProxy struct {
	cs *proxy.ProxyChannelTable
}

func (p *PaasProxy) Init() error {
	if !proxy.GConf.PAAS.Enable {
		return nil
	}
	for _, server := range proxy.GConf.PAAS.ServerList {
		var channel proxy.ProxyChannel
		for i := 0; i < proxy.GConf.PAAS.ConnsPerServer; i++ {
			if strings.HasPrefix(server, "ws://") || strings.HasPrefix(server, "wss://") {
				channel = newWebsocketChannel(server, i)
			}
			//log.Printf("#####%s %d", server, itc)
			if nil != channel {
				p.cs.Add(channel)
			}
		}
	}
	return nil
}

func (p *PaasProxy) Features() proxy.Feature {
	var f proxy.Feature
	f.MaxRequestBody = -1
	return f
}

func (p *PaasProxy) Serve(session *proxy.ProxySession, ev event.Event) error {
	if nil == session.Channel {
		session.Channel = p.cs.Select()
		if session.Channel == nil {
			return fmt.Errorf("No proxy channel in PaasProxy")
		}
	}
	switch ev.(type) {
	case *event.TCPChunkEvent:
		session.Channel.Write(ev)
	case *event.TCPCloseEvent:
		session.Channel.Write(ev)
	case *event.HTTPRequestEvent:
		if strings.EqualFold(ev.(*event.HTTPRequestEvent).Method, "Connect") {
			session.Hijacked = true
			host := ev.(*event.HTTPRequestEvent).Headers.Get("Host")
			tcpOpen := &event.TCPOpenEvent{Addr: host}
			tcpOpen.SetId(ev.GetId())
			session.Channel.Write(tcpOpen)
			ok := &event.TCPChunkEvent{Content: []byte("HTTP/1.1 200 Connection established\r\n\r\n")}
			ok.SetId(ev.GetId())
			proxy.HandleEvent(ok)
		} else {
			session.Channel.Write(ev)
		}
	default:
		log.Printf("Invalid event type:%T to process", ev)
	}
	return nil

}

var mypaas PaasProxy

func init() {
	mypaas.cs = proxy.NewProxyChannelTable()
	proxy.RegisterProxy("PAAS", &mypaas)
}

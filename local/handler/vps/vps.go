package vps

import (
	"fmt"
	"log"
	"strings"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type VPSProxy struct {
	cs *proxy.RemoteChannelTable
}

func (p *VPSProxy) Destory() error {
	p.cs.StopAll()
	return nil
}

func (p *VPSProxy) Init() error {
	if !proxy.GConf.VPS.Enable {
		return nil
	}
	server := proxy.GConf.VPS.Server
	for i := 0; i < proxy.GConf.VPS.ConnsPerServer; i++ {
		channel, err := newTCPChannel(server, i)
		if nil != channel {
			p.cs.Add(channel)
		} else {
			log.Printf("Failed to init proxy channel for %s:%d with reason:%v", server, i, err)
		}
	}
	return nil
}

func (p *VPSProxy) Features() proxy.Feature {
	var f proxy.Feature
	f.MaxRequestBody = -1
	return f
}

func (p *VPSProxy) Serve(session *proxy.ProxySession, ev event.Event) error {
	if nil == session.Remote {
		session.Remote = p.cs.Select()
		if session.Remote == nil {
			return fmt.Errorf("No proxy channel in PaasProxy")
		}
	}
	switch ev.(type) {
	case *event.TCPChunkEvent:
		session.Remote.Write(ev)
	case *event.UDPEvent:
		session.Remote.Write(ev)
	case *event.TCPOpenEvent:
		session.Remote.Write(ev)
	case *event.TCPCloseEvent:
		session.Remote.Write(ev)
	case *event.HTTPRequestEvent:
		if strings.EqualFold(ev.(*event.HTTPRequestEvent).Method, "Connect") {
			session.Hijacked = true
			host := ev.(*event.HTTPRequestEvent).Headers.Get("Host")
			tcpOpen := &event.TCPOpenEvent{Addr: host}
			tcpOpen.SetId(ev.GetId())
			session.Remote.Write(tcpOpen)
		} else {
			session.Remote.Write(ev)
		}
	default:
		log.Printf("Invalid event type:%T to process", ev)
	}
	return nil

}

var myvps VPSProxy

func init() {
	myvps.cs = proxy.NewRemoteChannelTable()
	proxy.RegisterProxy("VPS", &myvps)
}

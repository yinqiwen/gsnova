package vps

import (
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type VPSProxy struct {
	cs   *proxy.RemoteChannelTable
	conf proxy.ProxyChannelConfig
}

func (p *VPSProxy) PrintStat(w io.Writer) {
	fmt.Fprintf(w, "ProxyChannel %s Stat:\n", p.conf.Name)
	p.cs.PrintStat(w)
}

func (p *VPSProxy) Config() *proxy.ProxyChannelConfig {
	return &p.conf
}

func (p *VPSProxy) Destory() error {
	if nil != p.cs {
		p.cs.StopAll()
	}
	return nil
}

func (p *VPSProxy) Init(conf proxy.ProxyChannelConfig) error {
	p.cs = proxy.NewRemoteChannelTable()
	for _, server := range conf.ServerList {
		for i := 0; i < conf.ConnsPerServer; i++ {
			if !strings.Contains(server, "://") {
				server = "tcp://" + server
			}
			channel, err := newTCPChannel(server, i, conf)
			if nil != channel {
				p.cs.Add(channel)
			} else {
				log.Printf("Failed to init proxy channel for %s:%d with reason:%v", server, i, err)
				if i == 0 {
					return fmt.Errorf("Failed to auth %s", server)
				}
			}
		}
	}
	p.conf = conf
	return nil
}

func (p *VPSProxy) Features() proxy.Feature {
	var f proxy.Feature
	f.MaxRequestBody = -1
	return f
}

func (p *VPSProxy) Serve(session *proxy.ProxySession, ev event.Event) error {
	if nil == session.Remote {
		session.SetRemoteChannel(p.cs.Select())
		//session.Remote = p.cs.Select()
		if session.Remote == nil {
			session.Close()
			return fmt.Errorf("No proxy channel in PaasProxy")
		}
	}
	switch ev.(type) {
	case *event.TCPChunkEvent:
		session.Remote.Write(ev)
	case *event.ConnTestEvent:
		session.Remote.Write(ev)
	case *event.UDPEvent:
		session.Remote.Write(ev)
	case *event.TCPOpenEvent:
		session.Remote.Write(ev)
	case *event.ConnCloseEvent:
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
		session.Close()
		log.Printf("Invalid event type:%T to process", ev)
	}
	return nil

}

var myvps VPSProxy

func init() {
	//myvps.cs = proxy.NewRemoteChannelTable()
	proxy.RegisterProxyType("VPS", &VPSProxy{})
}

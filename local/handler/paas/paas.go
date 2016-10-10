package paas

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/proxy"
)

//var paasLocalProxyUrl *url.URL

type PaasProxy struct {
	cs   *proxy.RemoteChannelTable
	conf proxy.ProxyChannelConfig
}

func (p *PaasProxy) PrintStat(w io.Writer) {
	fmt.Fprintf(w, "ProxyChannel %s Stat:\n", p.conf.Name)
	p.cs.PrintStat(w)
}

func (p *PaasProxy) Config() *proxy.ProxyChannelConfig {
	return &p.conf
}

// func newRemoteChannel(server string, idx int, paasClient *http.Client, conf proxy.ProxyChannelConfig) (*proxy.RemoteChannel, error) {
// 	if strings.HasPrefix(server, "ws://") || strings.HasPrefix(server, "wss://") {
// 		return newWebsocketChannel(server, idx, conf)
// 	} else if strings.HasPrefix(server, "http://") || strings.HasPrefix(server, "https://") {
// 		return newHTTPChannel(server, idx, paasClient, conf)
// 	}
// 	return nil, fmt.Errorf("Not supported url:%s", server)
// }

func (p *PaasProxy) Destory() error {
	if nil != p.cs {
		p.cs.StopAll()
	}
	return nil
}

func (p *PaasProxy) Init(conf proxy.ProxyChannelConfig) error {
	p.conf = conf
	p.cs = proxy.NewRemoteChannelTable()
	paasHttpClient, err := proxy.NewHTTPClient(&p.conf)
	if nil != err {
		return err
	}
	for _, server := range conf.ServerList {
		for i := 0; i < conf.ConnsPerServer; i++ {
			var channel *proxy.RemoteChannel
			var err error
			if strings.HasPrefix(server, "ws://") || strings.HasPrefix(server, "wss://") {
				channel, err = newWebsocketChannel(server, i, conf, paasHttpClient.Transport.(*http.Transport).Dial)
			} else if strings.HasPrefix(server, "http://") || strings.HasPrefix(server, "https://") {
				channel, err = newHTTPChannel(server, i, paasHttpClient, conf)
			} else {
				err = fmt.Errorf("Not supported url:%s", server)
			}
			if nil != err {
				log.Printf("[ERROR]Failed to connect [%d]%s for reason:%v", i, server, err)
				if i == 0 {
					return fmt.Errorf("Failed to auth %s", server)
				}
				continue
			}
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

//var mypaas PaasProxy

func init() {
	//mypaas.cs = proxy.NewRemoteChannelTable()
	proxy.RegisterProxyType("PAAS", &PaasProxy{})
}

package paas

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/gsnova/local/proxy"
)

func paasDial(network, addr string) (net.Conn, error) {
	host, port, _ := net.SplitHostPort(addr)
	if port == "443" && len(proxy.GConf.PAAS.SNIProxy) > 0 {
		//host = hosts.GetHost(hosts.SNIProxy)
		host = hosts.GetHost(proxy.GConf.PAAS.SNIProxy)
		addr = host + ":" + port
	}
	log.Printf("[PAAS]Connect %s:%s", host, port)
	return netx.DialTimeout(network, addr, 3*time.Second)
}

type PaasProxy struct {
	cs *proxy.RemoteChannelTable
}

func newRemoteChannel(server string, idx int) (*proxy.RemoteChannel, error) {
	if strings.HasPrefix(server, "ws://") || strings.HasPrefix(server, "wss://") {
		return newWebsocketChannel(server, idx)
	} else if strings.HasPrefix(server, "http://") || strings.HasPrefix(server, "https://") {
		return newHTTPChannel(server, idx)
	}
	return nil, fmt.Errorf("Not supported url:%s", server)
}

func (p *PaasProxy) Destory() error {
	p.cs.StopAll()
	return nil
}

func (p *PaasProxy) Init() error {
	if !proxy.GConf.PAAS.Enable {
		return nil
	}
	for _, server := range proxy.GConf.PAAS.ServerList {
		for i := 0; i < proxy.GConf.PAAS.ConnsPerServer; i++ {
			channel, err := newRemoteChannel(server, i)
			if nil != err {
				log.Printf("[ERROR]Failed to connect [%d]%s for reason:%v", i, server, err)
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

var mypaas PaasProxy

func init() {
	mypaas.cs = proxy.NewRemoteChannelTable()
	tr := &http.Transport{
		Dial:                  paasDial,
		DisableCompression:    true,
		MaxIdleConnsPerHost:   2 * int(proxy.GConf.PAAS.ConnsPerServer),
		ResponseHeaderTimeout: 15 * time.Second,
	}
	paasHttpClient = &http.Client{}
	paasHttpClient.Timeout = 20 * time.Second
	paasHttpClient.Transport = tr
	proxy.RegisterProxy("PAAS", &mypaas)
}

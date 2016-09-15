package paas

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/hosts"
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
	readTimeout := conf.ReadTimeout
	if 0 == readTimeout {
		readTimeout = 30
	}

	paasDial := func(network, addr string) (net.Conn, error) {
		host, port, _ := net.SplitHostPort(addr)
		if port == "443" && len(conf.SNIProxy) > 0 {
			//host = hosts.GetHost(hosts.SNIProxy)
			host = hosts.GetHost(conf.SNIProxy)
			addr = host + ":" + port
		}
		if net.ParseIP(host) == nil {
			tcpaddr, err := netx.Resolve("tcp", addr)
			if nil != err {
				return nil, err
			}
			addr = tcpaddr.String()
			//addr = net.JoinHostPort(host, port)
		}
		log.Printf("[PAAS]Connect %s", addr)
		dailTimeout := conf.DialTimeout
		if 0 == dailTimeout {
			dailTimeout = 5
		}
		return netx.DialTimeout(network, addr, time.Duration(dailTimeout)*time.Second)
	}

	tr := &http.Transport{
		Dial:                  paasDial,
		DisableCompression:    true,
		MaxIdleConnsPerHost:   2 * int(conf.ConnsPerServer),
		ResponseHeaderTimeout: time.Duration(readTimeout) * time.Second,
	}
	if len(conf.SNI) > 0 {
		tlscfg := &tls.Config{}
		tlscfg.InsecureSkipVerify = true
		tlscfg.ServerName = conf.SNI[0]
		tr.TLSClientConfig = tlscfg
	}
	if len(conf.HTTPProxy) > 0 {
		proxyUrl, err := url.Parse(conf.HTTPProxy)
		if nil != err {
			return err
		}
		//paasLocalProxyUrl = proxyUrl
		tr.Proxy = http.ProxyURL(proxyUrl)
	}
	paasHttpClient := &http.Client{}
	paasHttpClient.Timeout = tr.ResponseHeaderTimeout
	paasHttpClient.Transport = tr
	for _, server := range conf.ServerList {
		for i := 0; i < conf.ConnsPerServer; i++ {
			var channel *proxy.RemoteChannel
			var err error
			if strings.HasPrefix(server, "ws://") || strings.HasPrefix(server, "wss://") {
				channel, err = newWebsocketChannel(server, i, conf, paasDial)
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

//var mypaas PaasProxy

func init() {
	//mypaas.cs = proxy.NewRemoteChannelTable()
	proxy.RegisterProxyType("PAAS", &PaasProxy{})
}

package gae

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type GAEProxy struct {
	cs               *proxy.ProxyChannelTable
	injectRangeRegex []*regexp.Regexp
}

func (p *GAEProxy) Init() error {
	if !proxy.GConf.PAAS.Enable {
		return nil
	}
	for _, server := range proxy.GConf.GAE.ServerList {
		channel := newHTTPChannel(server)
		if nil != channel {
			p.cs.Add(channel)
		}
	}
	p.injectRangeRegex, _ = proxy.NewRegex(proxy.GConf.GAE.InjectRange)
	return nil
}
func (p *GAEProxy) Features() proxy.Feature {
	var f proxy.Feature
	f.MaxRequestBody = 768 * 1024
	return f
}

func (p *GAEProxy) Serve(session *proxy.ProxySession, ev event.Event) error {
	if nil == session.Channel {
		session.Channel = p.cs.Select()
		if session.Channel == nil {
			return fmt.Errorf("No proxy channel in PaasProxy")
		}
	}
	switch ev.(type) {
	case *event.TCPCloseEvent:
		//session.Channel.Write(ev)
	case *event.HTTPRequestEvent:
		req := ev.(*event.HTTPRequestEvent)
		if strings.EqualFold(ev.(*event.HTTPRequestEvent).Method, "Connect") {
			session.SSLHijacked = true
			host := ev.(*event.HTTPRequestEvent).Headers.Get("Host")
			tcpOpen := &event.TCPOpenEvent{Addr: host}
			tcpOpen.SetId(ev.GetId())
			session.Channel.Write(tcpOpen)
			ok := &event.TCPChunkEvent{Content: []byte("HTTP/1.1 200 Connection established\r\n\r\n")}
			ok.SetId(ev.GetId())
			proxy.HandleEvent(ok)
		} else {
			if session.SSLHijacked {
				req.URL = "https://" + req.Headers.Get("Host") + req.URL
			} else {
				req.URL = "http://" + req.Headers.Get("Host") + req.URL
				defer session.Close()
			}
			rangeFetch := false
			rangeHeader := req.Headers.Get("Range")
			if strings.EqualFold(req.Method, "Get") && proxy.MatchRegexs(req.GetHost(), p.injectRangeRegex) {
				rangeFetch = true
			}
			if len(rangeHeader) > 0 {
				rangeFetch = true
			}
			if !rangeFetch {
				rev, err := session.Channel.Write(req)
				if nil != err {
					log.Printf("[ERROR]%v", err)
					session.Close()
					return err
				}
				switch rev.(type) {
				case *event.ErrorEvent:
					if rev.(*event.ErrorEvent).Code == event.ErrTooLargeResponse {
						rangeFetch = true
					} else {
						log.Printf("[ERROR]:%d(%s)", rev.(*event.ErrorEvent).Code, rev.(*event.ErrorEvent).Reason)
						session.Close()
						return nil
					}
				case *event.HTTPResponseEvent:
					proxy.HandleEvent(rev)
					return nil
				default:
					log.Printf("Invalid event type:%T to process", ev)
					session.Close()
					return nil
				}
			}
			if rangeFetch {
				fetcher := &proxy.RangeFetcher{
					SingleFetchLimit:  256 * 1024,
					ConcurrentFetcher: 3,
					C:                 session.Channel,
				}
				err := fetcher.Fetch(req)
				if nil != err {
					log.Printf("[ERROR]%v", err)
				}
			}

			return nil
		}
	default:
		log.Printf("Invalid event type:%T to process", ev)
	}
	return nil

}

var mygae GAEProxy

func init() {
	mygae.cs = proxy.NewProxyChannelTable()
	proxy.RegisterProxy("GAE", &mygae)
}

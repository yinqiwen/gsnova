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
	cs               *proxy.RemoteChannelTable
	injectRangeRegex []*regexp.Regexp
}

func (p *GAEProxy) Name() string {
	return "GAE"
}

func (p *GAEProxy) Init() error {
	if !proxy.GConf.GAE.Enable {
		return nil
	}
	err := initGAEClient()
	if nil != err {
		return err
	}
	for _, server := range proxy.GConf.GAE.ServerList {
		channel, err := newHTTPChannel(server)
		if nil != err {
			log.Printf("[ERROR]Failed to connect %s for reason:%v", server, err)
			continue
		}
		if nil != channel {
			p.cs.Add(channel)
		}
	}
	p.injectRangeRegex, _ = proxy.NewRegex(proxy.GConf.GAE.InjectRange)
	return nil
}

func (p *GAEProxy) Destory() error {
	return nil
}
func (p *GAEProxy) Features() proxy.Feature {
	var f proxy.Feature
	f.MaxRequestBody = 768 * 1024
	return f
}

func (p *GAEProxy) Serve(session *proxy.ProxySession, ev event.Event) error {
	if nil == session.Remote {
		session.Remote = p.cs.Select()
		if session.Remote == nil {
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
			return nil
		} else {
			if session.SSLHijacked {
				req.URL = "https://" + req.Headers.Get("Host") + req.URL
			} else {
				req.URL = "http://" + req.Headers.Get("Host") + req.URL
				defer session.Close()

			}
			rangeFetch := false
			rangeHeader := req.Headers.Get("Range")
			if len(rangeHeader) > 0 {
				rangeFetch = true
			}
			if !rangeFetch && strings.EqualFold(req.Method, "Get") && len(p.injectRangeRegex) > 0 && proxy.MatchRegexs(req.GetHost(), p.injectRangeRegex) {
				rangeFetch = true
			}
			adjustResp := func(r *event.HTTPResponseEvent) {
				r.Headers.Del("Transfer-Encoding")
				lenh := r.Headers.Get("Content-Length")
				if len(lenh) == 0 {
					r.Headers.Set("Content-Length", fmt.Sprintf("%d", len(r.Content)))
				}
			}

			if !rangeFetch {
				rev, err := session.Remote.Request(req)
				if nil != err {
					log.Printf("[ERROR]%v", err)
					session.Close()
					return err
				}
				rev.SetId(req.GetId())
				switch rev.(type) {
				case *event.NotifyEvent:
					rerr := rev.(*event.NotifyEvent)
					if rerr.Code == event.ErrTooLargeResponse {
						log.Printf("[Range]Recv too large response")
						rangeFetch = true
					} else {
						log.Printf("[ERROR]:%d(%s)", rerr.Code, rerr.Reason)
						session.Close()
						return nil
					}
				case *event.HTTPResponseEvent:
					res := rev.(*event.HTTPResponseEvent)
					//log.Printf("#### %s  %d %v %d", req.URL, res.StatusCode, res.Headers, len(res.Content))
					adjustResp(res)
					//res.Headers.Set("Content-Length", fmt.Sprintf("%d", len(res.Content)))
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
					C:                 session.Remote,
				}
				rev, err := fetcher.Fetch(req)
				if nil != err {
					log.Printf("[ERROR]%v", err)
				}
				if nil != rev {
					adjustResp(rev)
					proxy.HandleEvent(rev)
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

	mygae.cs = proxy.NewRemoteChannelTable()
	mygae.Init()
	proxy.RegisterProxy(&mygae)
}

package direct

import (
	"io"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type RejectProxy struct {
	conf proxy.ProxyChannelConfig
}

func (p *RejectProxy) PrintStat(w io.Writer) {
}

func (p *RejectProxy) Config() *proxy.ProxyChannelConfig {
	return &p.conf
}

func (p *RejectProxy) Init(conf proxy.ProxyChannelConfig) error {
	p.conf = conf
	return nil
}
func (p *RejectProxy) Destory() error {
	return nil
}
func (p *RejectProxy) Features() proxy.Feature {
	var f proxy.Feature
	f.MaxRequestBody = -1
	return f
}

func (p *RejectProxy) Serve(session *proxy.ProxySession, ev event.Event) error {
	if _, ok := ev.(*event.HTTPRequestEvent); ok {
		forbidden := &event.HTTPResponseEvent{}
		forbidden.SetId(ev.GetId())
		forbidden.StatusCode = 404
		proxy.HandleEvent(forbidden)
	} else {
		session.Close()
	}
	return nil
}

func init() {
	proxy.RegisterProxyType("REJECT", &RejectProxy{})
}

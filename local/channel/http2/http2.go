package http2

import (
	"net/url"

	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type HTTP2Proxy struct {
}

func (p *HTTP2Proxy) Features() proxy.ProxyFeatureSet {
	return proxy.ProxyFeatureSet{
		AutoExpire: true,
		Pingable:   true,
	}
}

func (tc *HTTP2Proxy) CreateMuxSession(server string, conf *proxy.ProxyChannelConfig) (mux.MuxSession, error) {
	rurl, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	conn, err := proxy.DialServerByConf(server, conf)
	if err != nil {
		return nil, err
	}
	//log.Printf("Connect %s success.", server)
	return mux.NewHTTP2ClientMuxSession(conn, rurl.Host)
}

func init() {
	proxy.RegisterProxyType("http2", &HTTP2Proxy{})
}

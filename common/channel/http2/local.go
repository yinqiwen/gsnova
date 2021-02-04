package http2

import (
	"net/url"

	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/mux"
)

type HTTP2Proxy struct {
}

func (p *HTTP2Proxy) Features() channel.FeatureSet {
	return channel.FeatureSet{
		AutoExpire: true,
		Pingable:   true,
	}
}

func (tc *HTTP2Proxy) CreateMuxSession(server string, conf *channel.ProxyChannelConfig) (mux.MuxSession, error) {
	rurl, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	conn, err := channel.DialServerByConf(server, conf)
	if err != nil {
		return nil, err
	}
	//log.Printf("Connect %s success.", server)
	return mux.NewHTTP2ClientMuxSession(conn, rurl.Host)
}

func init() {
	channel.RegisterLocalChannelType("http2", &HTTP2Proxy{})
}

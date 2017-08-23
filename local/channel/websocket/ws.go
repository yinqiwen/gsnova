package websocket

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/local/proxy"
	"github.com/yinqiwen/pmux"
)

type WebsocketProxy struct {
}

func (ws *WebsocketProxy) CreateMuxSession(server string, conf *proxy.ProxyChannelConfig) (mux.MuxSession, error) {
	u, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	u.Path = "/ws"
	wsDialer := &websocket.Dialer{}
	wsDialer.NetDial = proxy.NewDialByConf(conf)
	if len(conf.Proxy) > 0 {
		proxyUrl, err := url.Parse(conf.Proxy)
		if nil != err {
			return nil, err
		}
		wsDialer.Proxy = http.ProxyURL(proxyUrl)
	}
	if len(conf.SNI) > 0 {
		tlscfg := &tls.Config{}
		tlscfg.InsecureSkipVerify = true
		tlscfg.ServerName = conf.SNI[0]
		wsDialer.TLSClientConfig = tlscfg
	}
	c, _, err := wsDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("dial websocket error:%v", err)
		return nil, err
	}
	log.Printf("Connect %s success.", server)
	ps, err := pmux.Client(&mux.WsConn{Conn: c}, proxy.InitialPMuxConfig())
	if nil != err {
		return nil, err
	}
	return &mux.ProxyMuxSession{Session: ps}, nil
}

func init() {
	proxy.RegisterProxyType("ws", &WebsocketProxy{})
	proxy.RegisterProxyType("wss", &WebsocketProxy{})
}

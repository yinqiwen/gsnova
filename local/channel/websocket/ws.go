package websocket

import (
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/local/proxy"
	"github.com/yinqiwen/pmux"
)

type WebsocketProxy struct {
}

func (p *WebsocketProxy) Features() proxy.ProxyFeatureSet {
	return proxy.ProxyFeatureSet{
		AutoExpire: true,
		Pingable:   true,
	}
}

func (ws *WebsocketProxy) CreateMuxSession(server string, conf *proxy.ProxyChannelConfig) (mux.MuxSession, error) {
	u, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	u.Path = "/ws"
	wsDialer := &websocket.Dialer{}
	wsDialer.NetDial = proxy.NewDialByConf(conf)
	wsDialer.TLSClientConfig = proxy.NewTLSConfig(conf)
	c, _, err := wsDialer.Dial(u.String(), nil)
	if err != nil {
		logger.Notice("dial websocket error:%v", err)
		return nil, err
	}
	logger.Debug("Connect %s success.", server)
	ps, err := pmux.Client(&mux.WsConn{Conn: c}, proxy.InitialPMuxConfig(conf))
	if nil != err {
		return nil, err
	}
	return &mux.ProxyMuxSession{Session: ps}, nil
}

func init() {
	proxy.RegisterProxyType("ws", &WebsocketProxy{})
	proxy.RegisterProxyType("wss", &WebsocketProxy{})
}

package websocket

import (
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/pmux"
)

type WebsocketProxy struct {
}

func (p *WebsocketProxy) Features() channel.FeatureSet {
	return channel.FeatureSet{
		AutoExpire: true,
		Pingable:   true,
	}
}

func (ws *WebsocketProxy) CreateMuxSession(server string, conf *channel.ProxyChannelConfig) (mux.MuxSession, error) {
	u, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	u.Path = "/ws"
	wsDialer := &websocket.Dialer{}
	wsDialer.NetDial = channel.NewDialByConf(conf, u.Scheme)
	wsDialer.TLSClientConfig = channel.NewTLSConfig(conf)
	c, _, err := wsDialer.Dial(u.String(), nil)
	if err != nil {
		logger.Notice("dial websocket error:%v %v", err, u.String())
		return nil, err
	}
	logger.Debug("Connect %s success.", server)
	ps, err := pmux.Client(&mux.WsConn{Conn: c}, channel.InitialPMuxConfig(&conf.Cipher))
	if nil != err {
		return nil, err
	}
	return &mux.ProxyMuxSession{Session: ps}, nil
}

func init() {
	channel.RegisterLocalChannelType("ws", &WebsocketProxy{})
	channel.RegisterLocalChannelType("wss", &WebsocketProxy{})
}

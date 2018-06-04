package tcp

import (
	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/pmux"
)

type TcpProxy struct {
	//proxy.BaseProxy
}

func (p *TcpProxy) Features() channel.FeatureSet {
	return channel.FeatureSet{
		AutoExpire: true,
		Pingable:   true,
	}
}

func (tc *TcpProxy) CreateMuxSession(server string, conf *channel.ProxyChannelConfig) (mux.MuxSession, error) {
	conn, err := channel.DialServerByConf(server, conf)
	if err != nil {
		return nil, err
	}

	ps, err := pmux.Client(conn, channel.InitialPMuxConfig(&conf.Cipher))
	if nil != err {
		return nil, err
	}
	logger.Info("TCP Session:%v", server)
	return &mux.ProxyMuxSession{Session: ps, NetConn: conn}, nil
}

func init() {
	channel.RegisterLocalChannelType("tcp", &TcpProxy{})
	channel.RegisterLocalChannelType("tls", &TcpProxy{})
}

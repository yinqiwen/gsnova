package tcp

import (
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/local/proxy"
	"github.com/yinqiwen/pmux"
)

type TcpProxy struct {
	//proxy.BaseProxy
}

func (p *TcpProxy) Features() proxy.ProxyFeatureSet {
	return proxy.ProxyFeatureSet{
		AutoExpire: true,
	}
}

func (tc *TcpProxy) CreateMuxSession(server string, conf *proxy.ProxyChannelConfig) (mux.MuxSession, error) {
	conn, err := proxy.DialServerByConf(server, conf)
	if err != nil {
		return nil, err
	}
	//log.Printf("Connect %s success.", server)
	ps, err := pmux.Client(conn, proxy.InitialPMuxConfig(conf))
	if nil != err {
		return nil, err
	}
	return &mux.ProxyMuxSession{Session: ps}, nil
}

func init() {
	proxy.RegisterProxyType("tcp", &TcpProxy{})
	proxy.RegisterProxyType("tls", &TcpProxy{})
}

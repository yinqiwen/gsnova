package quic

import (
	"crypto/tls"
	"log"
	"net"
	"net/url"

	quic "github.com/lucas-clemente/quic-go"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type QUICProxy struct {
	//proxy.BaseProxy
}

func (p *QUICProxy) Features() proxy.ProxyFeatureSet {
	return proxy.ProxyFeatureSet{
		AutoExpire: true,
	}
}

func (tc *QUICProxy) CreateMuxSession(server string, conf *proxy.ProxyChannelConfig) (mux.MuxSession, error) {
	rurl, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	hostport := rurl.Host
	tcpHost, tcpPort, _ := net.SplitHostPort(hostport)
	if net.ParseIP(tcpHost) == nil {
		iphost, err := proxy.DnsGetDoaminIP(tcpHost)
		if nil != err {
			return nil, err
		}
		hostport = net.JoinHostPort(iphost, tcpPort)
	}
	var quicSession quic.Session
	quicSession, err = quic.DialAddr(hostport, &tls.Config{InsecureSkipVerify: true}, nil)
	if err != nil {
		return nil, err
	}
	log.Printf("Connect %s success.", server)
	return &mux.QUICMuxSession{Session: quicSession}, nil
}

func init() {
	proxy.RegisterProxyType("quic", &QUICProxy{})
}

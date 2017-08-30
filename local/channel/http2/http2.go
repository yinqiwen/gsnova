package http2

import (
	"crypto/tls"
	"log"
	"net"
	"net/url"
	"time"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/common/netx"
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type HTTP2Proxy struct {
}

func (p *HTTP2Proxy) Features() proxy.ProxyFeatureSet {
	return proxy.ProxyFeatureSet{
		AutoExpire: true,
	}
}

func (tc *HTTP2Proxy) CreateMuxSession(server string, conf *proxy.ProxyChannelConfig) (mux.MuxSession, error) {
	dailTimeout := conf.DialTimeout
	if 0 == dailTimeout {
		dailTimeout = 5
	}
	rurl, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	hostport := rurl.Host
	tcpHost, tcpPort, _ := net.SplitHostPort(hostport)
	if len(tcpPort) == 0 {
		tcpPort = "443"
		tcpHost = rurl.Host
		hostport = hostport + ":443"
	}
	tlscfg := &tls.Config{}
	tlscfg.InsecureSkipVerify = true
	if net.ParseIP(tcpHost) == nil {
		tlscfg.ServerName = tcpHost
	}
	if len(conf.SNIProxy) > 0 && tcpPort == "443" && hosts.InHosts(conf.SNIProxy) {
		hostport = hosts.GetAddr(conf.SNIProxy, "443")
		tcpHost, _, _ = net.SplitHostPort(hostport)
		log.Printf("HTTP2 channel select SNIProxy %s to connect", hostport)
	}
	if net.ParseIP(tcpHost) == nil {
		iphost, err := proxy.DnsGetDoaminIP(tcpHost)
		if nil != err {
			return nil, err
		}
		hostport = net.JoinHostPort(iphost, tcpPort)
	}

	timeout := time.Duration(dailTimeout) * time.Second
	var conn net.Conn
	if len(conf.Proxy) > 0 {
		conn, err = helper.HTTPProxyDial(conf.Proxy, hostport, timeout)
	} else {
		conn, err = netx.DialTimeout("tcp", hostport, timeout)
	}
	if err != nil {
		return nil, err
	}

	//tlscfg.ServerName = "xx-gsnova.7e14.starter-us-west-2.openshiftapps.com"
	tlsconn := tls.Client(conn, tlscfg)
	err = tlsconn.Handshake()
	if err != nil {
		return nil, err
	}
	conn = tlsconn
	log.Printf("Connect %s success.", server)
	return mux.NewHTTP2ClientMuxSession(conn, rurl.Host)
}

func init() {
	proxy.RegisterProxyType("http2", &HTTP2Proxy{})
}

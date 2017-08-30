package tcp

import (
	"crypto/tls"
	"log"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/common/netx"
	"github.com/yinqiwen/gsnova/local/hosts"
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
	dailTimeout := conf.DialTimeout
	if 0 == dailTimeout {
		dailTimeout = 5
	}
	rurl, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	tlscfg := &tls.Config{InsecureSkipVerify: true}
	hostport := rurl.Host
	tcpHost, tcpPort, _ := net.SplitHostPort(hostport)
	if net.ParseIP(tcpHost) == nil {
		tlscfg.ServerName = tcpHost
		iphost, err := proxy.DnsGetDoaminIP(tcpHost)
		if nil != err {
			return nil, err
		}
		hostport = net.JoinHostPort(iphost, tcpPort)
	}

	if strings.EqualFold(rurl.Scheme, "tls") {
		if len(conf.SNIProxy) > 0 && tcpPort == "443" && hosts.InHosts(conf.SNIProxy) {
			hostport = hosts.GetAddr(conf.SNIProxy, "443")
			tcpHost, _, _ = net.SplitHostPort(hostport)
			log.Printf("TCP channel select SNIProxy %s to connect", hostport)
		}
	}

	timeout := time.Duration(dailTimeout) * time.Second
	var conn net.Conn
	if len(conf.Proxy) > 0 {
		conn, err = helper.HTTPProxyDial(conf.Proxy, hostport, timeout)
	} else {
		conn, err = netx.DialTimeout("tcp", hostport, timeout)
	}
	if strings.EqualFold(rurl.Scheme, "tls") && nil == err {
		tlsConn := tls.Client(conn, tlscfg)
		err = tlsConn.Handshake()
		conn = tlsConn
	}
	if err != nil {
		return nil, err
	}
	log.Printf("Connect %s success.", server)
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

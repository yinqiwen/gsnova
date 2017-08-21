package tcp

import (
	"crypto/tls"
	"log"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/gsnova/local/proxy"
	"github.com/yinqiwen/pmux"
)

type TcpProxy struct {
	proxy.BaseProxy
}

func (tc *TcpProxy) CreateMuxSession(server string) (proxy.MuxSession, error) {
	dailTimeout := tc.Conf.DialTimeout
	if 0 == dailTimeout {
		dailTimeout = 5
	}
	rurl, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	var tlscfg *tls.Config
	hostport := rurl.Host
	tcpHost, tcpPort, _ := net.SplitHostPort(hostport)
	if net.ParseIP(tcpHost) == nil {
		iphost, err := proxy.DnsGetDoaminIP(tcpHost)
		if nil != err {
			return nil, err
		}
		hostport = net.JoinHostPort(iphost, tcpPort)
	}

	if strings.EqualFold(rurl.Scheme, "tls") {
		tlscfg = &tls.Config{}
		tlscfg.ServerName = tcpHost
		if len(tc.Conf.SNIProxy) > 0 && tcpPort == "443" && hosts.InHosts(tc.Conf.SNIProxy) {
			hostport = hosts.GetAddr(tc.Conf.SNIProxy, "443")
			tcpHost, _, _ = net.SplitHostPort(hostport)
			log.Printf("TCP channel select SNIProxy %s to connect", hostport)
		}
	}

	//log.Printf("######%s %s", vpsHost, tc.hostport)
	timeout := time.Duration(dailTimeout) * time.Second
	var conn net.Conn
	if len(tc.Conf.Proxy) > 0 {
		conn, err = helper.HTTPProxyDial(tc.Conf.Proxy, hostport, timeout)
	} else {
		conn, err = netx.DialTimeout("tcp", hostport, timeout)
	}
	if nil != tlscfg && nil == err {
		conn = tls.Client(conn, tlscfg)
	}

	if err != nil {
		return nil, err
	}
	log.Printf("Connect %s success.", server)
	ps, err := pmux.Client(conn, nil)
	if nil != err {
		return nil, err
	}
	return &proxy.ProxyMuxSession{Session: ps}, nil
}

func init() {
	proxy.RegisterProxyType("TCP", &TcpProxy{})
}

package proxy

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/netx"
	"github.com/yinqiwen/gsnova/local/hosts"
)

func NewTLSConfig(conf *ProxyChannelConfig) *tls.Config {
	tlscfg := &tls.Config{}
	tlscfg.InsecureSkipVerify = true
	if len(conf.SNI) > 0 {
		tlscfg.ServerName = conf.SNI[0]
	}
	return tlscfg
}

func DialServerByConf(server string, conf *ProxyChannelConfig) (net.Conn, error) {
	rurl, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	hostport := rurl.Host
	tcpHost, tcpPort, err := net.SplitHostPort(hostport)
	if nil != err {
		switch rurl.Scheme {
		case "http", "ws", "tcp", "tcp4", "tcp6":
			tcpHost = rurl.Host
			tcpPort = "80"
		case "ssh":
			tcpPort = "22"
			tcpHost = rurl.Host
		case "http2", "https", "quic", "kcp", "tls", "wss":
			tcpHost = rurl.Host
			tcpPort = "443"
		default:
			return nil, fmt.Errorf("Invalid scheme:%s", rurl.Scheme)
		}
		hostport = net.JoinHostPort(tcpHost, tcpPort)
	}
	tlscfg := NewTLSConfig(conf)
	if len(tlscfg.ServerName) == 0 {
		if net.ParseIP(tcpHost) == nil {
			tlscfg.ServerName = tcpHost
		}
	}

	if len(conf.SNIProxy) > 0 && tcpPort == "443" {
		if net.ParseIP(conf.SNIProxy) == nil {
			if hosts.InHosts(conf.SNIProxy) {
				hostport = hosts.GetAddr(conf.SNIProxy, "443")
				tcpHost, _, _ = net.SplitHostPort(hostport)
			}
		} else {
			tcpHost = conf.SNIProxy
			hostport = net.JoinHostPort(tcpHost, tcpPort)
		}
	}
	var conn net.Conn
	dailTimeout := conf.DialTimeout
	if 0 == dailTimeout {
		dailTimeout = 5
	}
	timeout := time.Duration(dailTimeout) * time.Second
	if len(conf.Proxy) == 0 {
		if net.ParseIP(tcpHost) == nil {
			iphost, err := DnsGetDoaminIP(tcpHost)
			if nil != err {
				return nil, err
			}
			hostport = net.JoinHostPort(iphost, tcpPort)
		}
		conn, err = netx.DialTimeout("tcp", hostport, timeout)
	} else {
		conn, err = helper.ProxyDial(conf.Proxy, hostport, timeout)
	}
	if nil == err {
		switch rurl.Scheme {
		case "tls":
			fallthrough
		case "http2":
			tlsconn := tls.Client(conn, tlscfg)
			err = tlsconn.Handshake()
			if err != nil {
				logger.Notice("TLS Handshake Failed %v", err)
				return nil, err
			}
			conn = tlsconn
		}
	}
	if nil != err {
		logger.Notice("Connect %s failed with reason:%v.", server, err)
	} else {
		logger.Debug("Connect %s success.", server)
	}
	return conn, err
}

func NewDialByConf(conf *ProxyChannelConfig, scheme string) func(network, addr string) (net.Conn, error) {
	localDial := func(network, addr string) (net.Conn, error) {
		//log.Printf("Connect %s", addr)
		server := fmt.Sprintf("%s://%s", scheme, addr)
		return DialServerByConf(server, conf)
	}
	return localDial
}

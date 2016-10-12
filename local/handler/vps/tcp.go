package vps

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type tcpChannel struct {
	conf         proxy.ProxyChannelConfig
	rurl         *url.URL
	originAddr   string
	conn         net.Conn
	proxyChannel *proxy.RemoteChannel
	useSNIProxy  bool
}

func (tc *tcpChannel) ReadTimeout() time.Duration {
	readTimeout := tc.conf.ReadTimeout
	if 0 == readTimeout {
		readTimeout = 15
	}
	return time.Duration(readTimeout) * time.Second
}

func (tc *tcpChannel) SetCryptoCtx(ctx *event.CryptoContext) {
}
func (tc *tcpChannel) HandleCtrlEvent(ev event.Event) {
	switch ev.(type) {
	case *event.PortUnicastEvent:
		if tc.useSNIProxy {
			return
		}
		host, _, _ := net.SplitHostPort(tc.rurl.Host)
		port := ev.(*event.PortUnicastEvent).Port
		tc.rurl.Host = net.JoinHostPort(host, strconv.Itoa(int(port)))
		tc.proxyChannel.Addr = tc.rurl.String()
		log.Printf("VPS channel updated remote address to %s", tc.rurl.String())
	}
}

func (tc *tcpChannel) Open() error {
	dailTimeout := tc.conf.DialTimeout
	if 0 == dailTimeout {
		dailTimeout = 5
	}
	var tlscfg *tls.Config
	hostport := tc.rurl.Host
	vpsHost, vpsPort, _ := net.SplitHostPort(hostport)
	if strings.EqualFold(tc.rurl.Scheme, "tls") {
		tlscfg = &tls.Config{}
		tlscfg.ServerName = vpsHost
		if len(tc.conf.SNIProxy) > 0 && vpsPort == "443" && hosts.InHosts(tc.conf.SNIProxy) {
			hostport = hosts.GetAddr(tc.conf.SNIProxy, "443")
			vpsHost, _, _ = net.SplitHostPort(hostport)
			tc.useSNIProxy = true
			log.Printf("VPS channel select SNIProxy %s to connect", hostport)
		}
	}

	if net.ParseIP(vpsHost) == nil {
		iphost, err := proxy.DnsGetDoaminIP(vpsHost)
		if nil != err {
			return err
		}
		hostport = net.JoinHostPort(iphost, vpsPort)
	}
	//log.Printf("######%s %s", vpsHost, tc.hostport)
	timeout := time.Duration(dailTimeout) * time.Second
	var c net.Conn
	var err error
	if len(tc.conf.Proxy) > 0 {
		c, err = helper.HTTPProxyDial(tc.conf.Proxy, hostport, timeout)
	} else {
		c, err = netx.DialTimeout("tcp", hostport, timeout)
	}
	if nil != tlscfg && nil == err {
		c = tls.Client(c, tlscfg)
	}
	if err != nil {
		if tc.rurl.String() != tc.originAddr {
			tc.rurl, _ = url.Parse(tc.originAddr)
			tc.proxyChannel.Addr = tc.rurl.String()
		}
		log.Printf("###Failed to connect %s with err:%v", hostport, err)
		return err
	}
	tc.conn = c
	return nil
}

func (tc *tcpChannel) Closed() bool {
	return nil == tc.conn
}

func (tc *tcpChannel) Close() error {
	conn := tc.conn
	if nil != conn {
		conn.Close()
		tc.conn = nil
	}
	return nil
}

func (tc *tcpChannel) Request([]byte) ([]byte, error) {
	return nil, nil
}

func (tc *tcpChannel) Read(p []byte) (int, error) {
	conn := tc.conn
	if nil == conn {
		return 0, io.EOF
	}
	conn.SetReadDeadline(time.Now().Add(tc.ReadTimeout()))
	return conn.Read(p)
}

func (tc *tcpChannel) Write(p []byte) (n int, err error) {
	conn := tc.conn
	if nil == conn {
		return 0, io.EOF
	}
	return conn.Write(p)
}

func newTCPChannel(addr string, idx int, conf proxy.ProxyChannelConfig) (*proxy.RemoteChannel, error) {
	rc := &proxy.RemoteChannel{
		Addr:                addr,
		Index:               idx,
		DirectIO:            false,
		OpenJoinAuth:        true,
		WriteJoinAuth:       false,
		HeartBeatPeriod:     conf.HeartBeatPeriod,
		ReconnectPeriod:     conf.ReconnectPeriod,
		RCPRandomAdjustment: conf.RCPRandomAdjustment,
		SecureTransport:     strings.HasPrefix(addr, "tls://"),
	}
	var err error
	tc := new(tcpChannel)
	tc.originAddr = addr
	tc.rurl, err = url.Parse(addr)
	if nil != err {
		return nil, err
	}
	tc.conf = conf
	tc.proxyChannel = rc
	rc.C = tc

	err = rc.Init(idx == 0)
	if nil != err {
		return nil, err
	}
	return rc, nil
}

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
	addr         string
	originAddr   string
	conn         net.Conn
	proxyChannel *proxy.RemoteChannel
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
		host, _, _ := net.SplitHostPort(tc.addr)
		port := ev.(*event.PortUnicastEvent).Port
		tc.addr = net.JoinHostPort(host, strconv.Itoa(int(port)))
		tc.proxyChannel.Addr = tc.addr
		log.Printf("VPS channel updated remote address to %s", tc.addr)
	}
}

func (tc *tcpChannel) Open() error {
	dailTimeout := tc.conf.DialTimeout
	if 0 == dailTimeout {
		dailTimeout = 5
	}
	var tlscfg *tls.Config
	u, _ := url.Parse(tc.addr)
	hostport := u.Host
	vpsHost, vpsPort, _ := net.SplitHostPort(hostport)
	if strings.EqualFold(u.Scheme, "tls") {
		tlscfg = &tls.Config{}
		tlscfg.ServerName = vpsHost
		if len(tc.conf.SNIProxy) > 0 && vpsPort == "443" {
			vpsHost = hosts.GetHost(tc.conf.SNIProxy)
			hostport = vpsHost + ":443"
			log.Printf("VPS channel select SNIProxy %s to connect", vpsHost)
		}
	}

	if net.ParseIP(vpsHost) == nil {
		tcpaddr, err := netx.Resolve("tcp", hostport)
		if nil != err {
			return err
		}
		hostport = tcpaddr.String()
		//tc.hostport = net.JoinHostPort(vpsHost, vpsPort)
	}
	//log.Printf("######%s %s", vpsHost, tc.hostport)
	timeout := time.Duration(dailTimeout) * time.Second
	var c net.Conn
	var err error
	if len(tc.conf.HTTPProxy) > 0 {
		c, err = helper.HTTPProxyConn(tc.conf.HTTPProxy, hostport, timeout)
	} else {
		c, err = netx.DialTimeout("tcp", hostport, timeout)
	}
	if nil != tlscfg {
		c = tls.Client(c, tlscfg)
	}
	if err != nil {
		if tc.addr != tc.originAddr {
			tc.addr = tc.originAddr
			tc.proxyChannel.Addr = tc.addr
		}
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
	tc := new(tcpChannel)
	tc.addr = addr
	tc.originAddr = addr
	tc.conf = conf
	tc.proxyChannel = rc
	rc.C = tc

	err := rc.Init(idx == 0)
	if nil != err {
		return nil, err
	}
	return rc, nil
}

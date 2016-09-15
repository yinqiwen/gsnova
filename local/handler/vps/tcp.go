package vps

import (
	"io"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
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
	timeout := time.Duration(dailTimeout) * time.Second
	var c net.Conn
	var err error
	if len(tc.conf.HTTPProxy) > 0 {
		c, err = helper.HTTPProxyConn(tc.conf.HTTPProxy, tc.addr, timeout)
	} else {
		c, err = netx.DialTimeout("tcp", tc.addr, timeout)
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

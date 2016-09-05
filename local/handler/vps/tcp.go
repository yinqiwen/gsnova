package vps

import (
	"io"
	"net"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type tcpChannel struct {
	addr string
	conn net.Conn
}

func (tc *tcpChannel) ReadTimeout() time.Duration {
	readTimeout := proxy.GConf.VPS.ReadTimeout
	if 0 == readTimeout {
		readTimeout = 15
	}
	return time.Duration(readTimeout) * time.Second
}

func (tc *tcpChannel) SetCryptoCtx(ctx *event.CryptoContext) {
}

func (tc *tcpChannel) Open() error {
	dailTimeout := proxy.GConf.VPS.DialTimeout
	if 0 == dailTimeout {
		dailTimeout = 5
	}
	timeout := time.Duration(dailTimeout) * time.Second
	var c net.Conn
	var err error
	if len(proxy.GConf.VPS.HTTPProxy) > 0 {
		c, err = helper.HTTPProxyConn(proxy.GConf.VPS.HTTPProxy, tc.addr, timeout)
	} else {
		c, err = netx.DialTimeout("tcp", tc.addr, timeout)
	}
	if err != nil {
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

func newTCPChannel(addr string, idx int) (*proxy.RemoteChannel, error) {
	rc := &proxy.RemoteChannel{
		Addr:            addr,
		Index:           idx,
		DirectIO:        false,
		OpenJoinAuth:    true,
		WriteJoinAuth:   false,
		HeartBeatPeriod: proxy.GConf.VPS.HeartBeatPeriod,
		ReconnectPeriod: proxy.GConf.VPS.ReconnectPeriod,
	}
	tc := new(tcpChannel)
	tc.addr = addr
	rc.C = tc

	err := rc.Init(idx == 0)
	if nil != err {
		return nil, err
	}
	return rc, nil
}

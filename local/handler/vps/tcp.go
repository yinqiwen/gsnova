package vps

import (
	"io"
	"net"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type tcpChannel struct {
	addr string
	conn net.Conn
}

func (tc *tcpChannel) Open() error {
	c, err := netx.DialTimeout("tcp", tc.addr, 5*time.Second)
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
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
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
	rc := new(proxy.RemoteChannel)
	rc.Addr = addr
	rc.Index = idx
	tc := new(tcpChannel)
	tc.addr = addr
	rc.C = tc

	err := rc.Init()
	if nil != err {
		return nil, err
	}
	return rc, nil
}

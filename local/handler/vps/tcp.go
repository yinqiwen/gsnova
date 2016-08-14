package vps

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type tcpChannel struct {
	addr string
	conn net.Conn
}

func (tc *tcpChannel) Open(iv uint64) error {
	connAddr := tc.addr
	if len(proxy.GConf.VPS.HTTPProxy) > 0 {
		proxyURL, err := url.Parse(proxy.GConf.VPS.HTTPProxy)
		if nil != err {
			return err
		}
		connAddr = proxyURL.Host
	}
	dailTimeout := proxy.GConf.VPS.DialTimeout
	if 0 == dailTimeout {
		dailTimeout = 5
	}
	c, err := netx.DialTimeout("tcp", connAddr, time.Duration(dailTimeout)*time.Second)
	if err != nil {
		return err
	}
	if len(proxy.GConf.VPS.HTTPProxy) > 0 {
		connReq, _ := http.NewRequest("Connect", tc.addr, nil)
		err = connReq.Write(c)
		if err != nil {
			return err
		}
		connRes, err := http.ReadResponse(bufio.NewReader(c), connReq)
		if err != nil {
			return err
		}
		if nil != connRes.Body {
			connRes.Body.Close()
		}
		if connRes.StatusCode >= 300 {
			return fmt.Errorf("Invalid Connect response:%d", connRes.StatusCode)
		}
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
	readTimeout := proxy.GConf.VPS.ReadTimeout
	if 0 == readTimeout {
		readTimeout = 15
	}
	conn.SetReadDeadline(time.Now().Add(time.Duration(readTimeout) * time.Second))
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
		Addr:          addr,
		Index:         idx,
		DirectIO:      false,
		OpenJoinAuth:  true,
		WriteJoinAuth: false,
		HeartBeat:     true,
	}
	tc := new(tcpChannel)
	tc.addr = addr
	rc.C = tc

	err := rc.Init()
	if nil != err {
		return nil, err
	}
	return rc, nil
}

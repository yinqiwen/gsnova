package direct

import (
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type directStream struct {
	conn    net.Conn
	conf    *proxy.ProxyChannelConfig
	addr    string
	session *directMuxSession
}

func (tc *directStream) Auth(req *mux.AuthRequest) error {
	return nil
}

func (tc *directStream) Connect(network string, addr string) error {
	host, port, _ := net.SplitHostPort(addr)
	//log.Printf("Session:%d enter direct with host %s & event:%T", ev.GetId(), host, ev)

	if len(tc.conf.SNIProxy) > 0 && port == "443" && network == "tcp" && hosts.InHosts(tc.conf.SNIProxy) {
		host = tc.conf.SNIProxy
	}
	isIP := net.ParseIP(host) != nil
	if !isIP {
		host = hosts.GetHost(host)
	}

	if nil == tc.conf.ProxyURL() {
		addr = host + ":" + port
	} else {
		addr = tc.conf.ProxyURL().Host
	}
	connectHost, connectPort, _ := net.SplitHostPort(addr)
	if net.ParseIP(connectHost) == nil {
		iphost, err := proxy.DnsGetDoaminIP(connectHost)
		if nil != err {
			return err
		}
		addr = net.JoinHostPort(iphost, connectPort)
	}
	dailTimeout := tc.conf.DialTimeout
	if 0 == dailTimeout {
		dailTimeout = 5
	}
	//log.Printf("Session:%d connect %s:%s for %s %T %v %v %s", ev.GetId(), network, addr, host, ev, needHttpsConnect, conf.ProxyURL(), net.JoinHostPort(host, port))
	c, err := netx.DialTimeout(network, addr, time.Duration(dailTimeout)*time.Second)
	if nil != tc.conf.ProxyURL() && nil == err {
		if strings.HasPrefix(tc.conf.ProxyURL().Scheme, "socks") {
			err = helper.Socks5ProxyConnect(tc.conf.ProxyURL(), c, net.JoinHostPort(host, port))
		} else {
			err = helper.HTTPProxyConnect(tc.conf.ProxyURL(), c, "https://"+net.JoinHostPort(host, port))
		}
	}
	if nil != err {
		log.Printf("Failed to connect %s for %s with error:%v", addr, host, err)
		return err
	}

	tc.conn = c
	tc.addr = addr
	return nil
}

func (tc *directStream) ReadTimeout() time.Duration {
	readTimeout := tc.conf.ReadTimeout
	if 0 == readTimeout {
		readTimeout = 15
	}
	return time.Duration(readTimeout) * time.Second
}
func (tc *directStream) StreamID() uint32 {
	return 0
}

func (tc *directStream) Read(p []byte) (int, error) {
	if nil == tc.conn {
		return 0, io.EOF
	}
	return tc.conn.Read(p)
}
func (tc *directStream) Write(p []byte) (int, error) {
	if nil == tc.conn {
		return 0, io.EOF
	}
	return tc.conn.Write(p)
}

func (tc *directStream) Close() error {
	conn := tc.conn
	if nil != conn {
		conn.Close()
		tc.conn = nil
	}
	tc.session.closeStream(tc)
	return nil
}

type directMuxSession struct {
	conf         *proxy.ProxyChannelConfig
	streams      map[*directStream]bool
	streamsMutex sync.Mutex
}

func (tc *directMuxSession) closeStream(s *directStream) {
	tc.streamsMutex.Lock()
	defer tc.streamsMutex.Unlock()
	delete(tc.streams, s)
}

func (tc *directMuxSession) CloseStream(stream mux.MuxStream) error {
	//stream.Close()
	return nil
}

func (tc *directMuxSession) OpenStream() (mux.MuxStream, error) {
	tc.streamsMutex.Lock()
	defer tc.streamsMutex.Unlock()
	stream := &directStream{
		conf:    tc.conf,
		session: tc,
	}
	return stream, nil
}
func (tc *directMuxSession) AcceptStream() (mux.MuxStream, error) {
	return nil, nil
}

func (tc *directMuxSession) NumStreams() int {
	tc.streamsMutex.Lock()
	defer tc.streamsMutex.Unlock()
	return len(tc.streams)
}

func (tc *directMuxSession) Close() error {
	tc.streamsMutex.Lock()
	defer tc.streamsMutex.Unlock()
	for stream := range tc.streams {
		stream.Close()
	}
	tc.streams = make(map[*directStream]bool)
	return nil
}

type DirectProxy struct {
}

func (p *DirectProxy) CreateMuxSession(server string, conf *proxy.ProxyChannelConfig) (mux.MuxSession, error) {
	session := &directMuxSession{
		conf:    conf,
		streams: make(map[*directStream]bool),
	}
	return session, nil
}

func init() {
	proxy.RegisterProxyType("direct", &DirectProxy{})
}

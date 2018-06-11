package direct

import (
	"io"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/dns"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/hosts"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/common/netx"
)

type directStream struct {
	net.Conn
	conf         *channel.ProxyChannelConfig
	addr         string
	session      *directMuxSession
	proxyServer  string
	latestIOTime time.Time
}

func (tc *directStream) SetReadDeadline(t time.Time) error {
	if nil == tc.Conn {
		return io.EOF
	}
	return tc.Conn.SetReadDeadline(t)
}
func (tc *directStream) SetWriteDeadline(t time.Time) error {
	if nil == tc.Conn {
		return io.EOF
	}
	return tc.Conn.SetWriteDeadline(t)
}

func (tc *directStream) Auth(req *mux.AuthRequest) *mux.AuthResponse {
	return &mux.AuthResponse{Code: mux.AuthOK}
}

func (tc *directStream) Connect(network string, addr string, opt mux.StreamOptions) error {
	host, port, _ := net.SplitHostPort(addr)
	//log.Printf("Session:%d enter direct with host %s & event:%T", ev.GetId(), host, ev)

	if len(tc.conf.SNIProxy) > 0 && port == "443" && network == "tcp" && hosts.InHosts(tc.conf.SNIProxy) {
		host = tc.conf.SNIProxy
	}
	isIP := net.ParseIP(host) != nil
	if !isIP {
		host = hosts.GetHost(host)
	}

	var proxyURL *url.URL
	if nil == tc.conf.ProxyURL() && len(tc.proxyServer) == 0 {
		addr = host + ":" + port
	} else {
		if nil != tc.conf.ProxyURL() {
			addr = tc.conf.ProxyURL().Host
			proxyURL = tc.conf.ProxyURL()
		} else {
			u, err := url.Parse(tc.proxyServer)
			if nil != err {
				return err
			}
			addr = u.Host
			proxyURL = u
		}
	}
	connectHost, connectPort, _ := net.SplitHostPort(addr)
	if net.ParseIP(connectHost) == nil {
		iphost, err := dns.DnsGetDoaminIP(connectHost)
		if nil != err {
			return err
		}
		addr = net.JoinHostPort(iphost, connectPort)
	}
	//dailTimeout := tc.conf.DialTimeout
	if 0 == opt.DialTimeout {
		opt.DialTimeout = 5000
	}
	//log.Printf("Session:%d connect %s:%s for %s %T %v %v %s", ev.GetId(), network, addr, host, ev, needHttpsConnect, conf.ProxyURL(), net.JoinHostPort(host, port))
	c, err := netx.DialTimeout(network, addr, time.Duration(opt.DialTimeout)*time.Millisecond)
	if nil != proxyURL && nil == err {
		switch proxyURL.Scheme {
		case "http_proxy":
			fallthrough
		case "http":
			err = helper.HTTPProxyConnect(tc.conf.ProxyURL(), c, "https://"+net.JoinHostPort(host, port))
		case "socks":
			fallthrough
		case "socks4":
			fallthrough
		case "socks5":
			err = helper.Socks5ProxyConnect(tc.conf.ProxyURL(), c, net.JoinHostPort(host, port))
		}
	}
	if nil != err {
		logger.Error("Failed to connect %s for %s with error:%v", addr, host, err)
		return err
	}

	tc.Conn = c
	tc.addr = addr
	return nil
}

func (tc *directStream) StreamID() uint32 {
	return 0
}

func (s *directStream) LatestIOTime() time.Time {
	return s.latestIOTime
}

func (tc *directStream) Read(p []byte) (int, error) {
	if nil == tc.Conn {
		return 0, io.EOF
	}
	tc.latestIOTime = time.Now()
	return tc.Conn.Read(p)
}
func (tc *directStream) Write(p []byte) (int, error) {
	if nil == tc.Conn {
		return 0, io.EOF
	}
	tc.latestIOTime = time.Now()
	return tc.Conn.Write(p)
}

func (tc *directStream) Close() error {
	conn := tc.Conn
	if nil != conn {
		conn.Close()
		tc.Conn = nil
	}
	tc.session.closeStream(tc)
	return nil
}

type directMuxSession struct {
	conf         *channel.ProxyChannelConfig
	streams      map[*directStream]bool
	streamsMutex sync.Mutex
	proxyServer  string
}

func (s *directMuxSession) RemoteAddr() net.Addr {
	return nil
}
func (s *directMuxSession) LocalAddr() net.Addr {
	return nil
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
		conf:        tc.conf,
		session:     tc,
		proxyServer: tc.proxyServer,
	}
	return stream, nil
}
func (tc *directMuxSession) AcceptStream() (mux.MuxStream, error) {
	return nil, channel.ErrNotSupportedOperation
}

func (tc *directMuxSession) NumStreams() int {
	tc.streamsMutex.Lock()
	defer tc.streamsMutex.Unlock()
	return len(tc.streams)
}
func (tc *directMuxSession) Ping() (time.Duration, error) {
	return 0, nil
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
	proxyType string
}

func (p *DirectProxy) CreateMuxSession(server string, conf *channel.ProxyChannelConfig) (mux.MuxSession, error) {
	ps := server
	if len(p.proxyType) == 0 {
		ps = ""
	}
	session := &directMuxSession{
		conf:        conf,
		streams:     make(map[*directStream]bool),
		proxyServer: ps,
	}
	return session, nil
}

func (p *DirectProxy) Features() channel.FeatureSet {
	return channel.FeatureSet{
		AutoExpire: false,
		Pingable:   false,
	}
}

func init() {
	channel.RegisterLocalChannelType(channel.DirectChannelName, &DirectProxy{})
	channel.RegisterLocalChannelType("socks", &DirectProxy{"socks5"})
	channel.RegisterLocalChannelType("socks4", &DirectProxy{"socks4"})
	channel.RegisterLocalChannelType("socks5", &DirectProxy{"socks5"})
	channel.RegisterLocalChannelType("http_proxy", &DirectProxy{"http_proxy"})
}

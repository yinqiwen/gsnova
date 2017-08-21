package quic

import (
	"crypto/tls"
	"log"
	"net"
	"net/url"

	quic "github.com/lucas-clemente/quic-go"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type quicMuxSession struct {
	quic.Session
}

func (q *quicMuxSession) OpenStream() (proxy.MuxStream, error) {
	s, err := q.OpenStreamSync()
	if nil != err {
		return nil, err
	}
	return &proxy.ProxyMuxStream{s}, nil
}

func (q *quicMuxSession) NumStreams() int {
	return -1
}

func (q *quicMuxSession) Close() error {
	return q.Session.Close(nil)
}

type QUICProxy struct {
	proxy.BaseProxy
}

func (tc *QUICProxy) CreateMuxSession(server string) (proxy.MuxSession, error) {
	rurl, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	hostport := rurl.Host
	tcpHost, tcpPort, _ := net.SplitHostPort(hostport)
	if net.ParseIP(tcpHost) == nil {
		iphost, err := proxy.DnsGetDoaminIP(tcpHost)
		if nil != err {
			return nil, err
		}
		hostport = net.JoinHostPort(iphost, tcpPort)
	}
	var quicSession quic.Session
	quicSession, err = quic.DialAddr(hostport, &tls.Config{InsecureSkipVerify: true}, nil)
	if err != nil {
		return nil, err
	}
	log.Printf("Connect %s success.", server)
	return &quicMuxSession{Session: quicSession}, nil
}

func init() {
	proxy.RegisterProxyType("QUIC", &QUICProxy{})
}

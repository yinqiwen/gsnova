package direct

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/gsnova/local/proxy"
)

var directProxy = &DirectProxy{}

type directChannel struct {
	sid            uint32
	conn           net.Conn
	httpsProxyConn bool
	udpProxyConn   bool
	conf           *proxy.ProxyChannelConfig
	addr           string
}

func (tc *directChannel) Connect(network string, addr string) error {
	host, port, _ := net.SplitHostPort(addr)
	host := ""
	port := ""
	//log.Printf("Session:%d enter direct with host %s & event:%T", ev.GetId(), host, ev)
	if len(host) == 0 {
		return fmt.Errorf("Empty remote addr in event")
	}

	if len(conf.SNIProxy) > 0 && port == "443" && network == "tcp" && hosts.InHosts(conf.SNIProxy) {
		host = conf.SNIProxy
	}
	isIP := net.ParseIP(host) != nil
	useTLS := false
	if conf.ForceTLS && port == "80" && hosts.InHosts(host) {
		useTLS = true
	} else {
		useTLS = false
	}
	if !isIP {
		host = hosts.GetHost(host)
	}

	//log.Printf("Session:%d get host:%s", ev.GetId(), host)
	addr := ""
	if nil == conf.ProxyURL() {
		if useTLS {
			addr = host + ":443"
		} else {
			if len(port) > 0 {
				addr = host + ":" + port
			} else {
				if needHttpsConnect {
					addr = host + ":443"
				} else {
					addr = host + ":80"
				}
			}
		}
	} else {
		addr = conf.ProxyURL().Host
	}
	connectHost, connectPort, _ := net.SplitHostPort(addr)
	if net.ParseIP(connectHost) == nil {
		iphost, err := proxy.DnsGetDoaminIP(connectHost)
		if nil != err {
			return err
		}
		addr = net.JoinHostPort(iphost, connectPort)
	}
	dailTimeout := conf.DialTimeout
	if 0 == dailTimeout {
		dailTimeout = 5
	}
	//log.Printf("Session:%d connect %s:%s for %s %T %v %v %s", ev.GetId(), network, addr, host, ev, needHttpsConnect, conf.ProxyURL(), net.JoinHostPort(host, port))
	c, err := netx.DialTimeout(network, addr, time.Duration(dailTimeout)*time.Second)
	if nil != conf.ProxyURL() && nil == err {
		if strings.HasPrefix(conf.ProxyURL().Scheme, "socks") {
			err = helper.Socks5ProxyConnect(conf.ProxyURL(), c, net.JoinHostPort(host, port))
		} else {
			if needHttpsConnect {
				err = helper.HTTPProxyConnect(conf.ProxyURL(), c, "https://"+net.JoinHostPort(host, port))
			}
		}
	}
	if nil != err {
		log.Printf("Failed to connect %s for %s with error:%v", addr, host, err)
		return err
	}
	tc.sid = ev.GetId()
	tc.conn = c
	tc.httpsProxyConn = needHttpsConnect
	tc.udpProxyConn = network == "udp"
	tc.conf = conf
	tc.addr = addr
	//d := &directChannel{ev.GetId(), c, fromHttpsConnect, network == "udp", toReplaceSNI, false, nil}
	if useTLS {
		tlcfg := &tls.Config{}
		tlcfg.InsecureSkipVerify = true
		sniLen := len(conf.SNI)
		if sniLen > 0 {
			tlcfg.ServerName = conf.SNI[rand.Intn(sniLen)]
		}
		tlsconn := tls.Client(c, tlcfg)
		err = tlsconn.Handshake()
		if nil != err {
			log.Printf("Failed to handshake with %s", addr)
		}
		tc.conn = tlsconn
	}
	if tc.httpsProxyConn {
		ctx.Hijacked = true
		return nil
	}
	return nil
}

func (tc *directChannel) ReadTimeout() time.Duration {
	readTimeout := tc.conf.ReadTimeout
	if 0 == readTimeout {
		readTimeout = 15
	}
	return time.Duration(readTimeout) * time.Second
}

func (tc *directChannel) Read() (event.Event, error) {
	b := make([]byte, 8192)
	tc.conn.SetReadDeadline(time.Now().Add(tc.ReadTimeout()))
	n, err := tc.conn.Read(b)
	var ev event.Event
	if n > 0 {
		if !tc.udpProxyConn {
			ev = &event.TCPChunkEvent{Content: b[0:n]}
		} else {
			ev = &event.UDPEvent{Content: b[0:n]}
		}
		ev.SetId(tc.sid)
	}
	return ev, err
}
func (tc *directChannel) Write(ev event.Event, ctx *helper.ProxyContext) error {
	switch ev.(type) {
	case *event.TCPOpenEvent:
	case *event.HTTPRequestEvent:
	case *event.UDPEvent:
	default:
		tc.Close()
		return fmt.Errorf("Can NOT create direct channel by event:%T", ev)
	}
	if nil == tc.conn {
		err := tc.initByEvent(ev, &directProxy.conf, ctx)
		if nil != err {
			tc.Close()
			return err
		}
	}

	switch ev.(type) {
	case *event.UDPEvent:
		tc.conn.Write(ev.(*event.UDPEvent).Content)
	case *event.ConnCloseEvent:
		tc.Close()
	case *event.TCPOpenEvent:
	case *event.ConnTestEvent:
		//do nothing
	case *event.TCPChunkEvent:
		tc.conn.Write(ev.(*event.TCPChunkEvent).Content)
	case *event.HTTPRequestEvent:
		req := ev.(*event.HTTPRequestEvent)
		content := req.HTTPEncode()
		_, err := tc.conn.Write(content)
		if nil != err {
			// closeEv := &event.ConnCloseEvent{}
			// closeEv.SetId(ev.GetId())
			// proxy.HandleEvent(closeEv)
			return err
		}
		return nil
	default:
		log.Printf("Invalid event type:%T to process", ev)
	}
	return nil
}
func (tc *directChannel) SetReadDeadline(t time.Time) error {
	return nil
}
func (tc *directChannel) SetWriteDeadline(t time.Time) error {
	return nil
}

func (tc *directChannel) Close() error {
	conn := tc.conn
	if nil != conn {
		conn.Close()
		tc.conn = nil
	}
	return nil
}

type DirectProxy struct {
	conf proxy.ProxyChannelConfig
}

func (p *DirectProxy) PrintStat(w io.Writer) {
}

func (p *DirectProxy) CreateMuxSession(server string) (MuxSession, error) {
	return nil
}

func init() {
	proxy2.RegisterProxyType("DIRECT", directProxy)
}

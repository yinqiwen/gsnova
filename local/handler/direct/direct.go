package direct

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type directChannel struct {
	sid  uint32
	conn net.Conn
}

func (d *directChannel) read() {
	for {
		if nil == d.conn {
			return
		}
		d.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		b := make([]byte, 8192)
		n, err := d.conn.Read(b)
		if nil != err {
			closeEv := &event.TCPCloseEvent{}
			closeEv.SetId(d.sid)
			proxy.HandleEvent(closeEv)
			return
		}
		ev := &event.TCPChunkEvent{Content: b[0:n]}
		ev.SetId(d.sid)
		proxy.HandleEvent(ev)
	}
}

func (d *directChannel) Write(ev event.Event) (event.Event, error) {
	switch ev.(type) {
	case *event.TCPCloseEvent:
		d.conn.Close()
	case *event.TCPChunkEvent:
		d.conn.Write(ev.(*event.TCPChunkEvent).Content)
	case *event.HTTPRequestEvent:
		req := ev.(*event.HTTPRequestEvent)
		content := req.HTTPEncode()
		_, err := d.conn.Write(content)
		if nil != err {
			closeEv := &event.TCPCloseEvent{}
			closeEv.SetId(d.sid)
			proxy.HandleEvent(closeEv)
			return nil, err
		}
		return nil, nil
	default:
		log.Printf("Invalid event type:%T to process", ev)
	}
	return nil, nil
}

func newDirectChannel(req *event.HTTPRequestEvent, useTLS bool) (*directChannel, error) {
	host := req.GetHost()
	port := ""
	if strings.Contains(host, ":") {
		host, port, _ = net.SplitHostPort(host)
	}
	if strings.EqualFold(req.Method, "Connect") && hosts.InHosts(hosts.SNIProxy) {
		host = hosts.SNIProxy
	}
	host = hosts.GetHost(host)
	addr := host
	if len(port) > 0 {
		addr = addr + ":" + port
	} else {
		if strings.EqualFold(req.Method, "Connect") || useTLS {
			addr = addr + ":443"
		} else {
			addr = addr + ":80"
		}
	}

	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	log.Printf("Session:%d connect %s for %s", req.GetId(), addr, req.GetHost())
	if nil != err {
		log.Printf("Failed to connect %s for %s with error:%v", addr, req.GetHost(), err)
		return nil, err
	}

	d := &directChannel{req.GetId(), c}
	if useTLS && !strings.EqualFold(req.Method, "Connect") {
		tlcfg := &tls.Config{}
		tlcfg.InsecureSkipVerify = true
		tlsconn := tls.Client(c, tlcfg)
		err = tlsconn.Handshake()
		if nil != err {
			log.Printf("Failed to handshake with %s", addr)
		}
		d.conn = tlsconn
	}
	go d.read()
	return d, nil
}

type DirectProxy struct {
	useTLS bool
}

func (p *DirectProxy) Init() error {
	return nil
}
func (p *DirectProxy) Features() proxy.Feature {
	var f proxy.Feature
	f.MaxRequestBody = -1
	return f
}

func (p *DirectProxy) Serve(session *proxy.ProxySession, ev event.Event) error {
	if nil == session.Channel {
		if req, ok := ev.(*event.HTTPRequestEvent); ok {
			c, err := newDirectChannel(req, p.useTLS)
			if nil != err {
				return err
			}
			session.Channel = c
			if strings.EqualFold(req.Method, "Connect") {
				session.Hijacked = true
				return nil
			}
		} else {
			return fmt.Errorf("Can NOT create direct channel by event:%T", ev)
		}
	}
	if nil == session.Channel {
		return fmt.Errorf("No No")
	}
	session.Channel.Write(ev)
	return nil

}

var directProxy DirectProxy
var tlsDirectProy DirectProxy

func init() {
	tlsDirectProy.useTLS = true
	proxy.RegisterProxy("Direct", &directProxy)
	proxy.RegisterProxy("TLSDirect", &tlsDirectProy)
}

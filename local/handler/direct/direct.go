package direct

import (
	"log"
	"net"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type directChannel struct {
	conn net.Conn
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
		// session.Channel = p.cs.Select()
		// if session.Channel == nil {
		// 	return fmt.Errorf("No proxy channel in PaasProxy")
		// }
	}
	switch ev.(type) {
	case *event.TCPCloseEvent:
	//session.Channel.Write(ev)
	case *event.TCPChunkEvent:
	case *event.HTTPRequestEvent:
	default:
		log.Printf("Invalid event type:%T to process", ev)
	}
	return nil

}

var directProxy DirectProxy
var tlsDirectProy DirectProxy

func init() {
	proxy.RegisterProxy("Direct", &directProxy)
	proxy.RegisterProxy("TLSDirect", &tlsDirectProy)
}

package paas

import (
	"bytes"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type websocketChannel struct {
	conf proxy.ProxyChannelConfig
	url  string
	dial func(network, addr string) (net.Conn, error)

	conn    *websocket.Conn
	readbuf bytes.Buffer
}

func (tc *websocketChannel) ReadTimeout() time.Duration {
	readTimeout := tc.conf.ReadTimeout
	if 0 == readTimeout {
		readTimeout = 15
	}
	return time.Duration(readTimeout) * time.Second
}

func (tc *websocketChannel) Request([]byte) ([]byte, error) {
	return nil, nil
}

func (hc *websocketChannel) SetCryptoCtx(ctx *event.CryptoContext) {
}
func (hc *websocketChannel) HandleCtrlEvent(ev event.Event) {

}

func (wc *websocketChannel) Open() error {
	u, err := url.Parse(wc.url)
	if nil != err {
		return err
	}
	u.Path = "/ws"
	wsDialer := &websocket.Dialer{}
	wsDialer.NetDial = wc.dial
	if len(wc.conf.Proxy) > 0 {
		proxyUrl, err := url.Parse(wc.conf.Proxy)
		if nil != err {
			return err
		}
		wsDialer.Proxy = http.ProxyURL(proxyUrl)
	}
	if len(wc.conf.SNI) > 0 {
		tlscfg := &tls.Config{}
		tlscfg.InsecureSkipVerify = true
		tlscfg.ServerName = wc.conf.SNI[0]
		wsDialer.TLSClientConfig = tlscfg
	}
	c, _, err := wsDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("dial websocket error:%v", err)
		return err
	}
	log.Printf("Connect %s success.", wc.url)
	wc.conn = c
	return nil
}

func (wc *websocketChannel) Closed() bool {
	return nil == wc.conn
}

func (wc *websocketChannel) Close() error {
	c := wc.conn
	if nil != c {
		c.Close()
		wc.conn = nil
	}
	return nil
}

func (wc *websocketChannel) Read(p []byte) (int, error) {
	if wc.readbuf.Len() > 0 {
		return wc.readbuf.Read(p)
	}
	wc.readbuf.Reset()
	c := wc.conn
	if nil == c {
		return 0, io.EOF
	}
	c.SetReadDeadline(time.Now().Add(wc.ReadTimeout()))
	mt, data, err := c.ReadMessage()
	if err != nil {
		if err != io.EOF {
			log.Printf("Websocket read error:%v", err)
		}
		wc.Close()
		return 0, err
	}
	switch mt {
	case websocket.BinaryMessage:
		wc.readbuf.Write(data)
		return wc.readbuf.Read(p)
	default:
		log.Printf("Invalid websocket message type")
		wc.Close()
		return 0, io.EOF
	}
}

func (wc *websocketChannel) Write(p []byte) (n int, err error) {
	c := wc.conn
	if nil == c {
		return 0, io.EOF
	}
	err = c.WriteMessage(websocket.BinaryMessage, p)
	if nil != err {
		wc.Close()
		log.Printf("Failed to write websocket binary messgage:%v", err)
		return 0, err
	} else {
		return len(p), nil
	}
}

func newWebsocketChannel(addr string, idx int, conf proxy.ProxyChannelConfig, dial func(network, addr string) (net.Conn, error)) (*proxy.RemoteChannel, error) {
	rc := &proxy.RemoteChannel{
		Addr:                addr,
		Index:               idx,
		DirectIO:            false,
		OpenJoinAuth:        true,
		WriteJoinAuth:       false,
		ReconnectPeriod:     conf.ReconnectPeriod,
		RCPRandomAdjustment: conf.RCPRandomAdjustment,
		HeartBeatPeriod:     conf.HeartBeatPeriod,
		SecureTransport:     strings.HasPrefix(addr, "wss://"),
	}
	tc := new(websocketChannel)
	tc.url = addr
	rc.C = tc
	tc.conf = conf
	tc.dial = dial

	err := rc.Init(idx == 0)
	if nil != err {
		return nil, err
	}
	return rc, nil
}

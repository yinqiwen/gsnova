package paas

import (
	"bytes"
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type websocketChannel struct {
	url     string
	conn    *websocket.Conn
	readbuf bytes.Buffer
}

func (tc *websocketChannel) ReadTimeout() time.Duration {
	readTimeout := proxy.GConf.PAAS.WSReadTimeout
	if 0 == readTimeout {
		readTimeout = 15
	}
	return time.Duration(readTimeout) * time.Second
}

func (tc *websocketChannel) Request([]byte) ([]byte, error) {
	return nil, nil
}

func (wc *websocketChannel) Open(iv uint64) error {
	u, err := url.Parse(wc.url)
	if nil != err {
		return err
	}
	u.Path = "/ws"
	wsDialer := &websocket.Dialer{}
	wsDialer.NetDial = paasDial
	if nil != paasLocalProxyUrl {
		wsDialer.Proxy = http.ProxyURL(paasLocalProxyUrl)
	}
	if len(proxy.GConf.PAAS.SNI) > 0 {
		tlscfg := &tls.Config{}
		tlscfg.InsecureSkipVerify = true
		tlscfg.ServerName = proxy.GConf.PAAS.SNI
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

func newWebsocketChannel(addr string, idx int) (*proxy.RemoteChannel, error) {
	rc := &proxy.RemoteChannel{
		Addr:            addr,
		Index:           idx,
		DirectIO:        false,
		OpenJoinAuth:    true,
		WriteJoinAuth:   false,
		ReconnectPeriod: proxy.GConf.PAAS.WSReconnectPeriod,
	}
	tc := new(websocketChannel)
	tc.url = addr
	rc.C = tc

	err := rc.Init()
	if nil != err {
		return nil, err
	}
	return rc, nil
}

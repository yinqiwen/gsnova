package websocket

import (
	"bytes"
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/local/proxy"
	"github.com/yinqiwen/pmux"
)

type wsConn struct {
	*websocket.Conn
	readbuf bytes.Buffer
}

func (ws *wsConn) Write(p []byte) (int, error) {
	c := ws.Conn
	if nil == c {
		return 0, io.EOF
	}
	err := c.WriteMessage(websocket.BinaryMessage, p)
	if nil != err {
		log.Printf("Failed to write websocket binary messgage:%v", err)
		return 0, err
	}
	return len(p), nil
}

func (ws *wsConn) Read(p []byte) (int, error) {
	if nil == ws.Conn {
		return 0, io.EOF
	}
	if ws.readbuf.Len() > 0 {
		return ws.readbuf.Read(p)
	}
	ws.readbuf.Reset()
	c := ws.Conn
	if nil == c {
		return 0, io.EOF
	}
	mt, data, err := c.ReadMessage()
	if err != nil {
		return 0, err
	}
	switch mt {
	case websocket.BinaryMessage:
		ws.readbuf.Write(data)
		return ws.readbuf.Read(p)
	default:
		log.Printf("Invalid websocket message type")
		return 0, io.EOF
	}
}

type WebsocketProxy struct {
	proxy.BaseProxy
}

func (ws *WebsocketProxy) CreateMuxSession(server string) (proxy.MuxSession, error) {
	u, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	u.Path = "/ws"
	wsDialer := &websocket.Dialer{}
	wsDialer.NetDial = proxy.NewDialByConf(&ws.BaseProxy.Conf)
	if len(ws.Conf.Proxy) > 0 {
		proxyUrl, err := url.Parse(ws.Conf.Proxy)
		if nil != err {
			return nil, err
		}
		wsDialer.Proxy = http.ProxyURL(proxyUrl)
	}
	if len(ws.Conf.SNI) > 0 {
		tlscfg := &tls.Config{}
		tlscfg.InsecureSkipVerify = true
		tlscfg.ServerName = ws.Conf.SNI[0]
		wsDialer.TLSClientConfig = tlscfg
	}
	c, _, err := wsDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("dial websocket error:%v", err)
		return nil, err
	}
	log.Printf("Connect %s success.", server)
	ps, err := pmux.Client(&wsConn{Conn: c}, nil)
	if nil != err {
		return nil, err
	}
	return &proxy.ProxyMuxSession{Session: ps}, nil
}

func init() {
	proxy.RegisterProxyType("WEBSOCKET", &WebsocketProxy{})
}

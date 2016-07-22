package paas

import (
	"bytes"
	"crypto/tls"
	"io"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type websocketChannel struct {
	url      string
	idx      int64
	conn     *websocket.Conn
	ch       chan event.Event
	authCode int
}

func writeEvent(conn *websocket.Conn, ev event.Event) error {
	var buf bytes.Buffer
	event.EncodeEvent(&buf, ev)
	return conn.WriteMessage(websocket.BinaryMessage, buf.Bytes())
}

func (wc *websocketChannel) reopen() error {
	u, err := url.Parse(wc.url)
	if nil != err {
		return err
	}
	u.Path = "/ws"
	wsDialer := &websocket.Dialer{}
	if strings.HasPrefix(wc.url, "wss://") && hosts.InHosts(hosts.SNIProxy) {
		wsDialer.TLSClientConfig = &tls.Config{}
		wsDialer.NetDial = paasDial
	}

	c, _, err := wsDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("dial websocket error:%v", err)
		return err
	}
	auth := proxy.NewAuthEvent()
	auth.Index = wc.idx
	err = writeEvent(c, auth)
	if nil != err {
		return err
	}
	log.Printf("[%d]Connect %s success.", wc.idx, wc.url)
	wc.conn = c
	return nil
}

func (wc *websocketChannel) init() {
	go wc.processWrite()
	go wc.processRead()
}
func (wc *websocketChannel) close() {
	if nil != wc.conn {
		wc.conn.Close()
		wc.conn = nil
	}
}

func (wc *websocketChannel) processWrite() {
	var writeEv event.Event
	for {
		conn := wc.conn
		if nil == conn {
			time.Sleep(1 * time.Second)
			continue
		}
		if nil == writeEv {
			writeEv = <-wc.ch
		}
		if nil != writeEv {
			var buf bytes.Buffer
			event.EncodeEvent(&buf, writeEv)
			//log.Printf("####%d write %T", ev.GetId(), ev)
			err := conn.WriteMessage(websocket.BinaryMessage, buf.Bytes())
			if nil != err {
				wc.close()
				log.Printf("Failed to write websocket binary messgage:%v", err)
			} else {
				writeEv = nil
			}
		}
	}
}

func (wc *websocketChannel) processRead() {
	for {
		conn := wc.conn
		if nil == conn {
			wc.reopen()
			time.Sleep(1 * time.Second)
			continue
		}
		mt, data, err := conn.ReadMessage()
		if err != nil {
			if err != io.EOF {
				log.Printf("Websocket read error:%v", err)
			}
			wc.close()
			continue
		}
		buf := bytes.NewBuffer(nil)
		switch mt {
		case websocket.BinaryMessage:
			if buf.Len() == 0 {
				buf = bytes.NewBuffer(data)
			} else {
				buf.Write(data)
			}
			for buf.Len() > 0 {
				err, ev := event.DecodeEvent(buf)
				if nil != err {
					log.Printf("Failed to decode event for reason:%v", err)
					break
				}
				if wc.idx < 0 {
					if auth, ok := ev.(*event.ErrorEvent); ok {
						wc.authCode = int(auth.Code)
					} else {
						log.Printf("[ERROR]Expected error event for auth all connection, but got %T.", ev)
					}
					wc.close()
					return
				} else {
					proxy.HandleEvent(ev)
				}
			}
		default:
			log.Printf("Invalid websocket message type")
		}
	}
}

func (wc *websocketChannel) Write(ev event.Event) (event.Event, error) {
	wc.ch <- ev
	return nil, nil
}

func newWebsocketChannel(url string, idx int64) *websocketChannel {
	wc := new(websocketChannel)
	wc.url = url
	wc.idx = idx
	wc.authCode = -1
	wc.ch = make(chan event.Event, 1)
	wc.init()
	if wc.idx < 0 {
		start := time.Now()
		for wc.authCode != 0 {
			if time.Now().After(start.Add(5*time.Second)) || wc.authCode > 0 {
				log.Printf("Server:%s auth failed", wc.url)
				wc.close()
				return nil
			}
			time.Sleep(1 * time.Millisecond)
		}
		log.Printf("Server:%s authed", wc.url)
	}
	return wc
}

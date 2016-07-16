package paas

import (
	"bytes"
	"io"
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type websocketChannel struct {
	url       string
	idx       int
	conn      *websocket.Conn
	ch        chan event.Event
	reconnect bool
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
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("dial websocket error:%v", err)
		return err
	}
	var auth event.AuthEvent
	auth.Index = uint32(wc.idx)
	auth.User = proxy.GConf.User
	auth.Reauth = wc.reconnect
	wc.reconnect = true
	err = writeEvent(c, &auth)
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
				proxy.HandleEvent(ev)
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

func newWebsocketChannel(url string, idx int) *websocketChannel {
	wc := new(websocketChannel)
	wc.url = url
	wc.idx = idx
	wc.ch = make(chan event.Event, 1)
	wc.init()
	return wc
}

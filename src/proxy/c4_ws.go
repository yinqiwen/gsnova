package proxy

import (
	//	"bufio"
	"bytes"
	"code.google.com/p/go.net/websocket"
	"common"
	"crypto/rc4"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"event"
	"fmt"
	"log"
	"net"
	//	"net/http"
	"errors"
	"net/url"
	"strings"
	"time"
)

func wsReadTask(ws *websocket.Conn, ch chan event.Event) {
	cumulate := new(C4CumulateTask)
	cumulate.chunkLen = -1
	for {
		var data []byte
		if len(c4SessionTable) > 0 {
			ws.SetReadDeadline(time.Now().Add(time.Duration(c4_cfg.WSReadTimeout) * time.Second))
		} else {
			ws.SetReadDeadline(time.Now().Add(120 * time.Second))
		}
		err := websocket.Message.Receive(ws, &data)
		if nil == err {
			err = cumulate.fillReadContent(data)
		}
		if nil != err {
			ch <- nil
			log.Printf("Close websocket connection with response:%v\n", err)
			return
		}
	}
}

var c4WsChannelTable = make(map[string][]chan event.Event)

func initC4WebsocketChannel(server string) {
	maxConn := int(c4_cfg.MaxConn)
	c4WsChannelTable[server] = make([]chan event.Event, maxConn)
	for i := 0; i < maxConn; i++ {
		ch := make(chan event.Event, 1000)
		c4WsChannelTable[server][i] = ch
		go wsC4Routine(server, i, ch)
	}
}

func wsOfferEvent(server string, ev event.Event) {
	chs := c4WsChannelTable[server]
	index := int(ev.GetHash()) % len(chs)
	chs[index] <- wrapC4RequestEvent(ev)
}

func wsC4Routine(server string, index int, ch chan event.Event) error {
	var ws *websocket.Conn
	u, err := url.Parse(server)
	if nil != err {
		return err
	}
	if len(u.Path) == 0 {
		u.Path = "/"
	}
	if !strings.Contains(u.Host, ":") {
		if strings.EqualFold(u.Scheme, "ws") {
			u.Host = net.JoinHostPort(u.Host, "80")
		} else if strings.EqualFold(u.Scheme, "wss") {
			u.Host = net.JoinHostPort(u.Host, "443")
		} else {
			return errors.New(fmt.Sprintf("Invalid scheme for websocket:%s", u.Scheme))
		}
		server = u.String()
	}

	checkWsConnection := func() bool {
		if nil == ws {

			origin := strings.Replace(server, "wss://", "https://", 1)
			origin = strings.Replace(origin, "ws://", "http://", 1)
			cc, err := websocket.Dial(server, "", origin)
			if nil != err {
				log.Printf("[ERROR]Failed to create websocket connection with response:%v\n", err)
				return false
			}
			type C4Handshake struct {
				UserToken string
				Index     int
				RC4Key    string
			}
			tmp := []byte(common.RC4Key)
			cipher, _ := rc4.NewCipher(tmp)
			dst := make([]byte, len(tmp))
			cipher.XORKeyStream(dst, tmp)

			hs := C4Handshake{
				UserToken: userToken,
				Index:     index,
				RC4Key:    base64.StdEncoding.EncodeToString(dst),
			}
			b, err := json.Marshal(hs)
			if nil != err {
				log.Printf("[ERROR]Failed to encode handshake json:%v\n", err)
				return false
			}
			cc.Write(b)
			var actual_msg = make([]byte, 512)
			n, err := cc.Read(actual_msg)
			if err != nil {
				log.Printf("[ERROR]Failed to read handshake response:%v\n", err)
				return false
			}

			resp := string(actual_msg[0:n])
			resp = strings.TrimSpace(resp)
			if strings.EqualFold(resp, "OK") {
				log.Printf("Success establish websocket:%d for %s", index, server)
			} else {
				log.Printf("Failed to establish websocket connection for response %s\n", resp)
				return false
			}

			ws = cc
			go wsReadTask(cc, ch)
		}
		return true
	}
	checkWsConnection()
	for {
		select {
		case ev := <-ch:
			if nil == ev {
				if nil != ws {
					ws.Close()
					ws = nil
				}
				log.Printf("Lost websocket connection:%d\n", index)
				checkWsConnection()
				continue
			}
			buf := new(bytes.Buffer)
			event.EncodeEvent(buf, ev)
			chunkLen := int32(buf.Len())

			var lenheader bytes.Buffer
			binary.Write(&lenheader, binary.BigEndian, &chunkLen)
			var allbuf bytes.Buffer
			allbuf.Write(lenheader.Bytes())
			allbuf.Write(buf.Bytes())
			data := allbuf.Bytes()
			for {
				for !checkWsConnection() {
					time.Sleep(500 * time.Millisecond)
				}

				err := websocket.Message.Send(ws, data)
				//n, err := ws.Write(data)
				if nil != err {
					log.Printf("[ERROR]Failed to write websocket server:%v\n", err)
					ws.Close()
					ws = nil
					continue
				}
				//				if n < len(data) {
				//					log.Printf("[ERROR]Unsent bytes:%d\n", len(data)-n)
				//				}
				break
			}
		}
	}
	return nil
}

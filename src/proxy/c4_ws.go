package proxy

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"event"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func wsReadTask(ws net.Conn, ch chan event.Event) {
	cumulate := new(C4CumulateTask)
	cumulate.chunkLen = -1
	for {
		err := cumulate.fillContent(ws)
		if nil != err {
			ch <- nil
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
	var ws net.Conn
	u, err := url.Parse(server)
	if nil != err {
		return err
	}
	checkWsConnection := func() bool {
		if nil == ws {
			if len(u.Path) == 0 {
				u.Path = "/"
			}
			request := fmt.Sprintf("GET / HTTP/1.1\r\nUpgrade: WebSocket\r\nHost: %s\r\nConnection: Upgrade\r\nConnectionIndex:%d\r\nUserToken:%s\r\nKeep-Alive: %d\r\n\r\n",  u.Host, index, userToken, c4_cfg.WSConnKeepAlive)
			addr := u.Host
			if !strings.Contains(u.Host, ":") {
				addr = net.JoinHostPort(u.Host, "80")
			}
			c, err := net.Dial("tcp", addr)
			if nil != err {
				log.Printf("[ERROR]Failed to connect websocket server:%v\n", err)
				return false
			}
			c.Write([]byte(request))
			response, err := http.ReadResponse(bufio.NewReader(c), new(http.Request))
			//			tmpbuf := new(bytes.Buffer)
			//			if nil != response {
			//				response.Write(tmpbuf)
			//				log.Printf("Recv response:%s\n", tmpbuf.String())
			//			}
			if nil != err || response.StatusCode != 101 {
				log.Printf("[ERROR]Failed to handshake websocket server:%v with response:%v\n", err, response)
				return false
			}
			ws = c
			go wsReadTask(c, ch)
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
				checkWsConnection()
				log.Printf("Lost websocket connection:%d\n", index)
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
				n, err := ws.Write(data)
				if nil != err {
					log.Printf("[ERROR]Failed to write websocket server:%v\n", err)
					ws.Close()
					ws = nil
					continue
				}
				if n < len(data) {
					log.Printf("[ERROR]Unsent bytes:%d\n", len(data)-n)
				}
				break
			}
		}
	}
	return nil
}

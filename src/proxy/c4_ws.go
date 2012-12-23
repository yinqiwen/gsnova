package proxy

import (
	"bufio"
	"bytes"
	//"code.google.com/p/go.net/websocket"
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
	data := make([]byte, 8192)
	var buffer bytes.Buffer
	chunkLen := int32(-1)
	for {
		n, err := ws.Read(data)
		if nil != err {
			ch <- nil
			return
		}
		buffer.Write(data[0:n])
		for {
			if chunkLen < 0 && buffer.Len() >= 4 {
				err = binary.Read(&buffer, binary.BigEndian, &chunkLen)
				if nil != err {
					log.Printf("#################%v\n", err)
					break
				}
			}
			if chunkLen >= 0 && buffer.Len() >= int(chunkLen) {
				content := buffer.Next(int(chunkLen))
				tmp := bytes.NewBuffer(content)
				err, evv := event.DecodeEvent(tmp)
				if nil == err {
					evv = event.ExtractEvent(evv)
					c4, exist := c4SessionTable[evv.GetHash()]
					if !exist {
						if evv.GetType() != event.EVENT_TCP_CONNECTION_TYPE {
							log.Printf("[ERROR]No C4 session found for %d with type:%T\n", evv.GetHash(), evv)
						}
					} else {
						c4.handleTunnelResponse(c4.sess, evv)
					}
				} else {
					log.Printf("[ERROR]Decode event failed %v with content len:%d\n", err, chunkLen)
				}
				buffer.Truncate(buffer.Len())
				chunkLen = -1
			} else {
				break
			}
		}
	}
}

var c4WsChannelTable = make(map[string][]chan event.Event)

func initC4WebsocketChannel(server string) {
	maxConn := int(c4_cfg.MaxWSConn)
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
			request := fmt.Sprintf("GET %s HTTP/1.1\r\nUpgrade: WebSocket\r\nHost: %s\r\nConnection: Upgrade\r\nConnectionIndex:%d\r\nUserToken:%s\r\nSec-WebSocket-Key:null\r\nSec-WebSocket-Protocol: c4\r\nSec-WebSocket-Version: 13\r\n\r\n", u.Path, u.Host, index, userToken)
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

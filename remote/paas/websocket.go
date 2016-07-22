package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/common/event"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

// handleWebsocket connection. Update to
func websocketInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		//log.WithField("err", err).Println("Upgrading to websockets")
		http.Error(w, "Error Upgrading to websockets", 400)
		return
	}

	writeEvent := func(ev event.Event) error {
		var buf bytes.Buffer
		event.EncodeEvent(&buf, ev)
		return ws.WriteMessage(websocket.BinaryMessage, buf.Bytes())
	}
	//log.Printf("###Recv websocket connection")
	buf := bytes.NewBuffer(nil)
	ctx := &ConnContex{}
	wsClosed := false
	var queue *event.EventQueue

	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			if err != io.EOF {
				log.Printf("Websoket read error:%v", err)
			}
			wsClosed = true
			break
		}
		switch mt {
		case websocket.BinaryMessage:
			if buf.Len() == 0 {
				buf = bytes.NewBuffer(data)
			} else {
				buf.Write(data)
			}
			ress, err := handleRequestBuffer(buf, ctx)
			if nil != err {
				log.Printf("[ERROR]connection %s:%d error:%v", ctx.User, ctx.Index, err)
				ws.Close()
				wsClosed = true
			} else {
				if nil == queue && len(ctx.User) > 0 && ctx.Index >= 0 {
					queue = getEventQueue(ctx.User, ctx.Index, true)
					go func() {
						for !wsClosed {
							ev, err := queue.Peek(1 * time.Millisecond)
							if nil != err {
								continue
							}
							err = writeEvent(ev)
							if nil != err {
								log.Printf("Websoket write error:%v", err)
								return
							} else {
								queue.ReadPeek()
							}
						}
					}()
				}
				for _, res := range ress {
					writeEvent(res)
				}
			}
		default:
			log.Printf("Invalid websocket message type")
			ws.Close()
		}
	}
	wsClosed = true
	log.Printf("Close websocket connection:%d", ctx.Index)
	//ws.WriteMessage(websocket.CloseMessage, []byte{})
}

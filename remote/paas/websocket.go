package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/remote"
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
	authedUser := ""
	connIndex := 0
	wsClosed := false
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
			for buf.Len() > 0 {
				err, ev := event.DecodeEvent(buf)
				if nil != err {
					log.Printf("Failed to decode event for reason:%v", err)
					break
				}
				//log.Printf("Recv event:%T in session:%d", ev, ev.GetId())
				if auth, ok := ev.(*event.AuthEvent); ok {
					if len(authedUser) == 0 {
						authedUser = auth.User
						if !remote.ServerConf.VerifyUser(authedUser) {
							var authres event.ErrorEvent
							authres.Code = event.ErrAuthFailed
							authres.SetId(ev.GetId())
							writeEvent(&authres)
							wsClosed = true
							ws.Close()
							return
						}
						authedUser = authedUser + "@" + ev.(*event.AuthEvent).Mac
						connIndex = int(auth.Index)
						log.Printf("Recv connection:%d from user:%s", connIndex, authedUser)
						if auth.Index < 0 {
							removeProxySessionsByUser(authedUser)
							closeUserEventQueue(authedUser)
							authedUser = ""
							var authres event.ErrorEvent
							authres.Code = 0
							authres.SetId(ev.GetId())
							writeEvent(&authres)
							continue
						}
						queue := getEventQueue(authedUser, connIndex, true)
						go func() {
							for !wsClosed {
								ev, err := queue.Peek(1 * time.Second)
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
					} else {
						log.Printf("Duplicate auth event in same connection")
					}
					continue
				} else {
					if len(authedUser) == 0 {
						log.Printf("Auth event MUST be first event.")
						break
					}
				}
				session := getProxySessionByEvent(authedUser, connIndex, ev)
				if nil != session {
					session.offer(ev)
				} else {
					if _, ok := ev.(*event.TCPCloseEvent); !ok {
						log.Printf("No session:%d found for event %T", ev.GetId(), ev)
					}
				}
			}
		default:
			log.Printf("Invalid websocket message type")
			ws.Close()
		}
	}
	log.Printf("Close websocket connection:%d", connIndex)
	//ws.WriteMessage(websocket.CloseMessage, []byte{})
}

package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/remote"
)

func readRequestBuffer(r *http.Request) *bytes.Buffer {
	b := make([]byte, r.ContentLength)
	io.ReadFull(r.Body, b)
	r.Body.Close()
	reqbuf := bytes.NewBuffer(b)
	return reqbuf
}

// handleWebsocket connection. Update to
func httpInvoke(w http.ResponseWriter, r *http.Request) {
	ctx := remote.NewConnContext()
	writeEvent := func(ev event.Event) error {
		var buf bytes.Buffer
		event.EncryptEvent(&buf, ev, ctx.IV)
		_, err := w.Write(buf.Bytes())
		if nil == err {
			w.(http.Flusher).Flush()
		}
		return err
	}
	reqbuf := readRequestBuffer(r)

	ress, err := remote.HandleRequestBuffer(reqbuf, ctx)
	if nil != err {
		log.Printf("[ERROR]connection %s:%d error:%v with path:%s ", ctx.User, ctx.ConnIndex, err, r.URL.Path)
		w.WriteHeader(400)
	} else {
		w.WriteHeader(200)
		begin := time.Now()
		if strings.HasSuffix(r.URL.Path, "pull") {
			for _, res := range ress {
				writeEvent(res)
			}
			queue := remote.GetEventQueue(ctx.ConnId, true)
			for {
				if time.Now().After(begin.Add(10 * time.Second)) {
					log.Printf("Stop puller after 10s for conn:%d", ctx.ConnIndex)
					break
				}
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
		}
	}
}

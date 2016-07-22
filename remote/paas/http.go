package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
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
	writeEvent := func(ev event.Event) error {
		var buf bytes.Buffer
		chunklen := uint32(1)
		binary.Write(&buf, binary.BigEndian, chunklen)
		event.EncodeEvent(&buf, ev)
		var tmp bytes.Buffer
		chunklen = uint32(buf.Len())
		binary.Write(&tmp, binary.BigEndian, chunklen)
		copy(buf.Bytes()[0:4], tmp.Bytes()[0:4])

		_, err := w.Write(buf.Bytes())
		if nil == err {
			w.(http.Flusher).Flush()
		}
		return err
	}
	reqbuf := readRequestBuffer(r)
	ctx := &ConnContex{}
	ress, err := handleRequestBuffer(reqbuf, ctx)
	if nil != err {
		log.Printf("[ERROR]connection %s:%d error:%v", ctx.User, ctx.Index, err)
		w.WriteHeader(400)
	} else {
		w.WriteHeader(200)
		for _, res := range ress {
			writeEvent(res)
		}
		begin := time.Now()
		if strings.HasSuffix(r.URL.Path, "pull") {
			queue := getEventQueue(ctx.User, ctx.Index, true)
			for {
				if time.Now().After(begin.Add(10 * time.Second)) {
					log.Printf("Stop puller after 10s for conn:%d", ctx.Index)
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

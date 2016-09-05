package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/remote"
)

func handleRequestBody(r *http.Request, ctx *remote.ConnContext) ([]event.Event, error) {
	var rbuf bytes.Buffer
	var evs []event.Event
	var body io.Reader
	body = r.Body
	// if r.ContentLength <= 0 {
	// 	body = httputil.NewChunkedReader(body)
	// }
	reader := &helper.BufferChunkReader{body, nil}
	defer r.Body.Close()
	for {
		rbuf.Grow(8192)
		rbuf.ReadFrom(reader)
		ress, err := remote.HandleRequestBuffer(&rbuf, ctx)
		if nil != err {
			if err != event.EBNR {
				log.Printf("[ERROR]connection %s:%d error:%v", ctx.User, ctx.ConnIndex, err)
				return nil, err
			}
		} else {
			if len(ress) > 0 {
				evs = append(evs, ress...)
			}
		}
		if nil != reader.Err {
			break
		}
	}
	return evs, nil
}

// handleWebsocket connection. Update to
func httpInvoke(w http.ResponseWriter, r *http.Request) {
	ctx := remote.NewConnContext()
	writeEvents := func(evs []event.Event, buf *bytes.Buffer) error {
		if len(evs) > 0 {
			buf.Reset()
			for _, ev := range evs {
				if nil != ev {
					event.EncryptEvent(buf, ev, &ctx.CryptoContext)
				}
			}
			if buf.Len() > 0 {
				_, err := w.Write(buf.Bytes())
				if nil == err {
					w.(http.Flusher).Flush()
				}
				return err
			}
		}
		return nil
	}
	//reqbuf := readRequestBuffer(r)
	var wbuf bytes.Buffer
	ress, err := handleRequestBody(r, ctx)
	if nil != err {
		log.Printf("[ERROR]connection %s:%d error:%v with path:%s ", ctx.User, ctx.ConnIndex, err, r.URL.Path)
		w.WriteHeader(400)
	} else {
		w.WriteHeader(200)
		if strings.HasSuffix(r.URL.Path, "pull") {
			begin := time.Now()
			period, _ := strconv.Atoi(r.Header.Get("X-PullPeriod"))
			if period <= 0 {
				period = 15
			}
			writeEvents(ress, &wbuf)
			queue := remote.GetEventQueue(ctx.ConnId, true)
			defer remote.ReleaseEventQueue(queue)
			for {
				if time.Now().After(begin.Add(time.Duration(period) * time.Second)) {
					log.Printf("Stop puller after %ds for conn:%d", period, ctx.ConnIndex)
					break
				}
				evs, err := queue.PeekMulti(2, 1*time.Millisecond, false)
				if nil != err {
					continue
				}
				err = writeEvents(evs, &wbuf)
				if nil != err {
					log.Printf("HTTP write error:%v", err)
					return
				}
				queue.DiscardPeeks(false)
			}
		}
	}
}

package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"util"
)

const (
	STATE_WAIT_HEAD_RES      = 1
	STATE_WAIT_RANGE_GET_RES = 2
)

type rangeBody struct {
	c   chan []byte
	buf bytes.Buffer
}

func (r *rangeBody) Read(p []byte) (n int, err error) {
	if r.buf.Len() > 0 {
		return r.buf.Read(p)
	}
	select {
	case b := <-r.c:
		if nil == b {
			return -1, io.EOF
		}
		r.buf.Write(b)
		return r.buf.Read(p)
	}
	return -1, io.EOF
}

func (r *rangeBody) Close() error {
	r.c <- nil
	return nil
}

func newRangeBody() *rangeBody {
	body := new(rangeBody)
	body.c = make(chan []byte)
	return body
}

type rangeFetchTask struct {
	FetchLimit     int
	FetchWorkerNum int

	SessionID        uint32
	rangeWorker      int32
	contentBegin     int
	contentEnd       int
	rangeState       int
	rangePos         int
	expectedRangePos int
	originRangeHader string
	cursorMutex      sync.Mutex
	req              *http.Request
	res              *http.Response
	chunks           map[int][]byte
	closed           bool
}

func (r *rangeFetchTask) processRequest(req *http.Request) error {
	if !strings.EqualFold(req.Method, "GET") {
		return fmt.Errorf("Only GET request supported!")
	}
	r.req = req
	rangeHeader := req.Header.Get("range")
	r.contentEnd = -1
	r.contentBegin = 0
	r.rangePos = 0
	r.expectedRangePos = 0
	if r.FetchLimit == 0 {
		r.FetchLimit = 256 * 1024
	}
	if r.FetchWorkerNum == 0 {
		r.FetchWorkerNum = 1
	}
	r.chunks = make(map[int][]byte)
	if len(rangeHeader) > 0 {
		r.originRangeHader = rangeHeader
		r.contentBegin, r.contentEnd = util.ParseRangeHeaderValue(rangeHeader)
		r.rangePos = r.contentBegin
		r.expectedRangePos = r.rangePos
	}
	return nil
}

func (r *rangeFetchTask) Close() {
	if nil != r.res && nil != r.res.Body {
		r.res.Body.Close()
	}
	r.closed = true
}

func (r *rangeFetchTask) ProcessResponse(res *http.Response) error {
	if r.closed {
		return fmt.Errorf("Session[%d] already closed for handling range response.", r.SessionID)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Expected 2xx response, but got %d", res.StatusCode)
	}
	if r.contentEnd == -1 {
		r.contentEnd = int(res.ContentLength) - 1
	}
	switch r.rangeState {
	case STATE_WAIT_HEAD_RES:
		r.res = res
		if len(r.originRangeHader) > 0 {
			r.res.StatusCode = 206
			r.res.Status = ""
			r.res.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", r.contentBegin, r.contentEnd, res.ContentLength))
		}
		return nil
	case STATE_WAIT_RANGE_GET_RES:
		contentRange := res.Header.Get("Content-Range")
		start, end, _ := util.ParseContentRangeHeaderValue(contentRange)
		var tmp bytes.Buffer
		io.Copy(&tmp, res.Body)
		if tmp.Len() != end-start+1 {
			return fmt.Errorf("No full body readed for %s", contentRange)
		}
		r.chunks[start] = tmp.Bytes()
		log.Printf("Session[%d]Recv range chunk:%s", r.SessionID, contentRange)
		body := r.res.Body.(*rangeBody)
		for {
			if chunk, exist := r.chunks[r.expectedRangePos]; exist {
				delete(r.chunks, r.expectedRangePos)
				r.expectedRangePos += len(chunk)
				body.c <- chunk
			} else {
				if r.expectedRangePos < r.contentEnd {
					log.Printf("Session[%d]Expect range chunk:%d\n", r.SessionID, r.expectedRangePos)
				}
				break
			}
		}
	}
	return nil
}

func (r *rangeFetchTask) StartAync(req *http.Request, httpWrite func(*http.Request) error) error {
	r.processRequest(req)
	if len(r.originRangeHader) > 0 {
		if r.contentEnd > 0 && r.contentEnd-r.contentBegin < r.FetchLimit {
			return httpWrite(req)
		}
	}
	req.Method = "HEAD"
	r.rangeState = STATE_WAIT_HEAD_RES
	return httpWrite(req)
}

func (r *rangeFetchTask) Start(req *http.Request, fetch func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	r.processRequest(req)
	if len(r.originRangeHader) > 0 {
		if r.contentEnd > 0 && r.contentEnd-r.contentBegin < r.FetchLimit {
			return fetch(req)
		}
	}
	req.Method = "HEAD"
	r.rangeState = STATE_WAIT_HEAD_RES
	res, err := fetch(req)
	if nil != err {
		return res, err
	}
	err = r.ProcessResponse(res)
	if nil != err {
		return res, err
	}
	r.rangeState = STATE_WAIT_RANGE_GET_RES
	r.res.Body = newRangeBody()
	var loop_fetch func()
	var f func(int, int)

	f = func(begin, end int) {
		clonereq := *(r.req)
		clonereq.Method = "GET"
		rangeHeader := fmt.Sprintf("bytes=%d-%d", begin, end)
		clonereq.Header.Set("Range", rangeHeader)
		log.Printf("Session[%d]Fetch range:%s\n", r.SessionID, rangeHeader)
		res, err := fetch(&clonereq)
		//log.Printf("Fetch range res:%v\n", res)
		if nil == err && res.StatusCode == 206 {
			if nil != r.ProcessResponse(res) {
				r.Close()
			}
		} else {
			if nil == err && res.StatusCode == 302 {
				location := res.Header.Get("Location")
				if len(location) > 0 {
					clonereq.RequestURI = location
				}
			}
			//try again
			res, err = fetch(&clonereq)
			if nil != err {
				log.Printf("###Recv error:%v\n", err)
				r.Close()
				atomic.AddInt32(&r.rangeWorker, -1)
				return
			}
		}
		atomic.AddInt32(&r.rangeWorker, -1)
		loop_fetch()
	}
	loop_fetch = func() {
		for !r.closed && int(r.rangeWorker) < r.FetchWorkerNum && r.rangePos < r.contentEnd {
			r.cursorMutex.Lock()
			begin := r.rangePos
			end := r.rangePos + r.FetchLimit
			if end > r.contentEnd {
				end = r.contentEnd
			}
			r.rangePos = end + 1
			r.cursorMutex.Unlock()
			atomic.AddInt32(&r.rangeWorker, 1)
			go f(begin, end)
		}
	}
	loop_fetch()
	return res, nil
}

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
	STATE_WAIT_NORMAL_RES    = 0
	STATE_WAIT_HEAD_RES      = 1
	STATE_WAIT_RANGE_GET_RES = 2
)

type rangeAsyncNotify interface {
	onResponse(*http.Response)
}

type rangeBody struct {
	c      chan []byte
	buf    bytes.Buffer
	closed bool
}

func (r *rangeBody) Read(p []byte) (n int, err error) {
	if r.closed {
		return 0, io.EOF
	}
	if r.buf.Len() > 0 {
		return r.buf.Read(p)
	}
	r.buf.Reset()
	select {
	case b := <-r.c:
		if nil == b {
			r.buf.Reset()
			return 0, io.EOF
		}
		r.buf.Write(b)
		//log.Printf("###########buf len %d, p len:%d\n", r.buf.Len(), len(p))
		return r.buf.Read(p)
	}
	return 0, io.EOF
}

func (r *rangeBody) Close() error {
	if !r.closed {
		r.c <- nil
	}
	r.closed = true
	return nil
}

func newRangeBody() *rangeBody {
	body := new(rangeBody)
	body.c = make(chan []byte, 10)
	return body
}

type rangeFetchTask struct {
	FetchLimit     int
	FetchWorkerNum int
	TaskValidation func() bool

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
		r.chunks = make(map[int][]byte)
		r.res.Body.Close()
	}
	r.closed = true
}

func (r *rangeFetchTask) processResponse(res *http.Response) error {
	if r.closed {
		return fmt.Errorf("Session[%d] already closed for handling range response.", r.SessionID)
	}
	if nil != r.TaskValidation {
		if !r.TaskValidation() {
			r.Close()
			return fmt.Errorf("Task ternminated by callback")
		}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Expected 2xx response, but got %d", res.StatusCode)
	}
	if r.contentEnd == -1 {
		r.contentEnd = int(res.ContentLength) - 1
	}
	switch r.rangeState {
	case STATE_WAIT_NORMAL_RES:
		r.res = res
		return nil
	case STATE_WAIT_HEAD_RES:
		r.res = res
		r.req.Method = "GET"
		if len(r.originRangeHader) > 0 {
			r.res.StatusCode = 206
			r.res.Status = ""
			if res.ContentLength == 0 {
				res.ContentLength = int64(r.contentEnd - r.contentBegin + 1)
			}
			r.res.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", r.contentBegin, r.contentEnd, res.ContentLength))
		}
		r.res.Body = newRangeBody()
		return nil
	case STATE_WAIT_RANGE_GET_RES:
		contentRange := res.Header.Get("Content-Range")
		start, end, _ := util.ParseContentRangeHeaderValue(contentRange)
		var tmp bytes.Buffer
		io.Copy(&tmp, res.Body)
		if tmp.Len() != end-start+1 {
			return fmt.Errorf("No full body readed for %s", contentRange)
		}
		log.Printf("Session[%d]Recv range chunk:%s", r.SessionID, contentRange)
		body := r.res.Body.(*rangeBody)
		if start == r.expectedRangePos {
			r.expectedRangePos += tmp.Len()
			body.c <- tmp.Bytes()
		} else {
			r.chunks[start] = tmp.Bytes()
		}
		for {
			if chunk, exist := r.chunks[r.expectedRangePos]; exist {
				delete(r.chunks, r.expectedRangePos)
				r.expectedRangePos += len(chunk)
				body.c <- chunk
			} else {
				if r.expectedRangePos < r.contentEnd {
					log.Printf("Session[%d]Expect range chunk:%d\n", r.SessionID, r.expectedRangePos)
				} else {
					body.c <- nil
				}
				break
			}
		}
	}
	return nil
}

func (r *rangeFetchTask) ProcessAyncResponse(res *http.Response, httpWrite func(*http.Request) error) (*http.Response, error) {
	if r.rangeState == STATE_WAIT_RANGE_GET_RES && res.StatusCode == 302 {
		location := res.Header.Get("Location")
		xrange := res.Header.Get("X-Range")
		if len(location) > 0 && len(xrange) > 0 {
			r.req.RequestURI = location
			r.req.Header.Set("Range", xrange)
			httpWrite(r.req)
			return r.res, nil
		}
	}
	err := r.processResponse(res)
	if nil != err {
		r.Close()
		return r.res, err
	}
	if r.rangeState == STATE_WAIT_NORMAL_RES {
		return r.res, nil
	}
	var res *http.Response
	if r.rangeState == STATE_WAIT_RANGE_GET_RES {
		atomic.AddInt32(&r.rangeWorker, -1)
	} else {
		res = r.res
		r.rangeState = STATE_WAIT_RANGE_GET_RES
	}

	for !r.closed && int(r.rangeWorker) < r.FetchWorkerNum && r.rangePos < r.contentEnd && (r.rangePos-r.expectedRangePos) < (r.FetchLimit*r.FetchWorkerNum*2) {
		r.cursorMutex.Lock()
		begin := r.rangePos
		end := r.rangePos + r.FetchLimit - 1
		if end > r.contentEnd {
			end = r.contentEnd
		}
		r.rangePos = end + 1
		r.cursorMutex.Unlock()
		atomic.AddInt32(&r.rangeWorker, 1)
		rangeHeader := fmt.Sprintf("bytes=%d-%d", begin, end)
		r.req.Header.Set("Range", rangeHeader)
		r.req.Header.Set("X-Snova-HCE", "1")
		httpWrite(r.req)
	}
	return res, nil
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
	if res.StatusCode >= 300 {
		return res, nil
	}
	err = r.processResponse(res)
	if nil != err {
		return res, err
	}
	r.rangeState = STATE_WAIT_RANGE_GET_RES

	var loop_fetch func()
	var f func(int, int)

	f = func(begin, end int) {
		clonereq := *(r.req)
		clonereq.Header = make(http.Header)
		for k, vs := range r.req.Header {
			for _, v := range vs {
				clonereq.Header.Add(k, v)
			}
		}
		clonereq.Method = "GET"
		rangeHeader := fmt.Sprintf("bytes=%d-%d", begin, end)
		clonereq.Header.Set("Range", rangeHeader)
		log.Printf("Session[%d]Fetch range:%s\n", r.SessionID, rangeHeader)
		res, err := fetch(&clonereq)
		//log.Printf("Fetch range res:%v\n", res)
		if nil == err && res.StatusCode == 206 {
			err = r.processResponse(res)
			if nil != err {
				r.Close()
				log.Printf("Session[%d]Range Fetch:%s failed:%v\n", r.SessionID, rangeHeader, err)
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
		for !r.closed && int(r.rangeWorker) < r.FetchWorkerNum && r.rangePos < r.contentEnd && (r.rangePos-r.expectedRangePos) < r.FetchLimit*r.FetchWorkerNum*2 {
			r.cursorMutex.Lock()
			begin := r.rangePos
			end := r.rangePos + r.FetchLimit - 1
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

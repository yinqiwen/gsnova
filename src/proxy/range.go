package proxy

import (
	"fmt"
	"io"
	//"log"
	"net/http"
	"strings"
	"util"
)

const (
	STATE_WAIT_HEAD_RES = 1
)

type rangeBody struct {
	c chan []byte
}

func (r *rangeBody) Read(p []byte) (n int, err error) {
	select {
	case b := <-r.c:
		if nil == b {
			return -1, io.EOF
		}

	}
	return -1, io.EOF
}

func (r *rangeBody) Close() error {
	r.c <- nil
	return nil
}

type rangeFetchTask struct {
	FetchLimit     int
	FetchWorkerNum int

	rangeWorker      int
	contentBegin     int
	contentEnd       int
	rangeState       int
	originRangeHader string
	req              *http.Request
	res              *http.Response
}

func (r *rangeFetchTask) processRequest(req *http.Request) error {
	if !strings.EqualFold(req.Method, "GET") {
		return fmt.Errorf("Only GET request supported!")
	}
	r.req = req
	rangeHeader := req.Header.Get("range")
	r.contentEnd = -1
	r.contentBegin = 0
	if len(rangeHeader) > 0 {
		r.originRangeHader = rangeHeader
		r.contentBegin, r.contentEnd = util.ParseRangeHeaderValue(rangeHeader)
	}
	return nil
}

func (r *rangeFetchTask) ProcessResponse(res *http.Response) error {
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Expected 2xx response")
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
	}
	return nil
}

func (r *rangeFetchTask) StartAync(req *http.Request, writer func(*http.Request) error) error {
	r.processRequest(req)
	req.Method = "HEAD"
	r.rangeState = STATE_WAIT_HEAD_RES
	writer(req)
	return nil
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

	for r.rangeWorker < r.FetchWorkerNum {

	}
	return nil, nil
}

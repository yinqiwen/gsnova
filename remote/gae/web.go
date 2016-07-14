package gae

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yinqiwen/gsnova/common/event"

	"appengine"
	"appengine/urlfetch"
)

func fetch(context appengine.Context, ev *event.HTTPRequestEvent) event.Event {
	errorResponse := new(event.HTTPResponseEvent)
	req := ev.ToRequest("http")

	if req == nil {
		errorResponse.Status = 400
		fillErrorResponse(errorResponse, "Invalid fetch url:"+ev.Url)
		return errorResponse
	}
	var t urlfetch.Transport
	t.Context = context
	t.Deadline, _ = time.ParseDuration("10s")
	t.AllowInvalidServerCertificate = true
	//t := &transport
	//t := &urlfetch.Transport{context, 0, true}
	retryCount := 2
	for retryCount > 0 {
		resp, err := t.RoundTrip(req)
		if err == nil {
			res := buildHTTPResponseEvent(resp)
			if res.Status == 302 {
				rangeHeader := req.Header.Get("Range")
				if len(rangeHeader) > 0 {
					res.AddHeader("X-Range", rangeHeader)
				}
			}
			return res
		}
		context.Errorf("Failed to fetch URL[%s] for reason:%v", ev.Url, err)
		retryCount--
		if strings.EqualFold(req.Method, "GET") && strings.Contains(err.Error(), "RESPONSE_TOO_LARGE") {
			rangeLimit := Cfg.RangeFetchLimit
			rangestart := 0
			rangeheader := req.Header.Get("Range")
			if len(rangeheader) > 0 {
				rangestart, _ = util.ParseRangeHeaderValue(rangeheader)
			}
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rangestart, rangeLimit-1))
		}
		if strings.Contains(err.Error(), "RESPONSE_TOO_LARGE") {
			time.Sleep(1 * time.Second)
			return Fetch(context, ev)
		}
	}
	errorResponse.Status = 408
	fillErrorResponse(errorResponse, "Fetch timeout for url:"+ev.Url)
	rangeHeader := req.Header.Get("Range")
	if len(rangeHeader) > 0 {
		errorResponse.SetHeader("X-Range", rangeHeader)
	}
	return errorResponse

}

func httpInvoke(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, r.ContentLength)
	r.Body.Read(b)
	buf := bytes.NewBuffer(b)

}

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Hello, world!")
}

func init() {
	http.HandleFunc("/invoke", httpInvoke)
}

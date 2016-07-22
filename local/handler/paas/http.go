package paas

import (
	"bytes"
	"encoding/binary"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/proxy"
)

var paasHttpClient *http.Client

type httpChannel struct {
	url      string
	idx      int64
	ch       chan event.Event
	authCode int
	running  bool
}

func (hc *httpChannel) init() {
	hc.running = true
	go hc.processWrite()
	go hc.processRead()
}
func (hc *httpChannel) close() {
	hc.running = false
	hc.Write(nil)
}

func (hc *httpChannel) processWrite() {
	u, _ := url.Parse(hc.url)
	u.Path = "/http/push"
	auth := proxy.NewAuthEvent()
	auth.Index = int64(hc.idx)

	readWriteEv := func(buf *bytes.Buffer) int {
		sev := <-hc.ch
		if nil != sev {
			event.EncodeEvent(buf, sev)
			return 1
		}
		return 0
	}

	for hc.running {
		var buf bytes.Buffer
		event.EncodeEvent(&buf, auth)
		count := 0

		if len(hc.ch) == 0 {
			count += readWriteEv(&buf)
		} else {
			for len(hc.ch) > 0 {
				count += readWriteEv(&buf)
			}
		}

		if !hc.running && count == 0 {
			return
		}
		req := &http.Request{
			Method:        "POST",
			URL:           u,
			ProtoMajor:    1,
			ProtoMinor:    1,
			Host:          u.Host,
			Header:        make(http.Header),
			Body:          ioutil.NopCloser(&buf),
			ContentLength: int64(buf.Len()),
		}
		req.Close = false
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Content-Type", "image/jpeg")
		if len(proxy.GConf.UserAgent) > 0 {
			req.Header.Set("User-Agent", proxy.GConf.UserAgent)
		}
		s := time.Now()
		response, err := paasHttpClient.Do(req)
		if nil != err || response.StatusCode != 200 {
			log.Printf("Failed to push data to PAAS:%s for reason:%v or res:%v", hc.url, err, response)
		} else {
			hc.processHttpBody(response)
		}
		e := time.Now()
		log.Printf("[%d]Push %d events to %s cost %v", hc.idx, count, hc.url, e.Sub(s))
	}
}

func (hc *httpChannel) processRead() {
	u, _ := url.Parse(hc.url)
	u.Path = "/http/pull"
	for hc.running && hc.idx >= 0 {
		auth := proxy.NewAuthEvent()
		auth.Index = int64(hc.idx)
		var buf bytes.Buffer
		event.EncodeEvent(&buf, auth)
		req := &http.Request{
			Method:        "POST",
			URL:           u,
			ProtoMajor:    1,
			ProtoMinor:    1,
			Host:          u.Host,
			Header:        make(http.Header),
			Body:          ioutil.NopCloser(&buf),
			ContentLength: int64(buf.Len()),
		}
		req.Close = false
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Content-Type", "image/jpeg")
		if len(proxy.GConf.UserAgent) > 0 {
			req.Header.Set("User-Agent", proxy.GConf.UserAgent)
		}
		response, err := paasHttpClient.Do(req)
		if nil != err || response.StatusCode != 200 {
			log.Printf("Failed to pull data from PAAS:%s for reason:%v or res:%v", hc.url, err, response)
		} else {
			hc.processHttpBody(response)
		}
	}
}

func (hc *httpChannel) Write(ev event.Event) (event.Event, error) {
	hc.ch <- ev
	return nil, nil
}

func (hc *httpChannel) processHttpBody(res *http.Response) {
	if nil == res.Body {
		return
	}
	lenbuf := make([]byte, 4)
	defer res.Body.Close()
	data := make([]byte, 8192)
	var buf bytes.Buffer
	for {
		n, err := res.Body.Read(data)
		buf.Write(data[0:n])
		for buf.Len() > 0 {
			if buf.Len() < 4 {
				break
			}
			tmp := bytes.NewBuffer(buf.Bytes()[0:4])
			chunklen := uint32(1)
			binary.Read(tmp, binary.BigEndian, &chunklen)
			if chunklen > uint32(buf.Len()) {
				break
			}
			buf.Read(lenbuf)
			err, ev := event.DecodeEvent(&buf)
			if nil != err {
				log.Printf("Failed to decode event for reason:%v", err)
				return
			}
			if hc.idx < 0 {
				if auth, ok := ev.(*event.ErrorEvent); ok {
					hc.authCode = int(auth.Code)
				} else {
					log.Printf("[ERROR]Expected error event for auth all connection, but got %T.", ev)
				}
				hc.close()
				return
			} else {
				proxy.HandleEvent(ev)
			}
		}
		if nil != err {
			return
		}
	}
}

func newHTTPChannel(url string, idx int64) *httpChannel {
	hc := new(httpChannel)
	hc.url = url
	hc.idx = idx
	hc.authCode = -1
	hc.ch = make(chan event.Event, 10)
	hc.init()
	if hc.idx < 0 {
		hc.Write(nil)
		start := time.Now()
		for hc.authCode != 0 {
			if time.Now().After(start.Add(5*time.Second)) || hc.authCode > 0 {
				log.Printf("Server:%s auth failed", hc.url)
				hc.close()
				return nil
			}
			time.Sleep(1 * time.Millisecond)
		}
		log.Printf("Server:%s authed", hc.url)
	}
	return hc
}

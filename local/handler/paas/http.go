package paas

import (
	"bytes"
	"io"
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
	addr    string
	idx     int
	pushurl *url.URL
	pullurl *url.URL

	iv      uint64
	rbody   io.ReadCloser
	pulling bool
}

func (hc *httpChannel) Open(iv uint64) error {
	hc.iv = iv

	return hc.pull()
}

func (hc *httpChannel) Closed() bool {
	return nil == hc.rbody
}

func (tc *httpChannel) Request([]byte) ([]byte, error) {
	return nil, nil
}

func (hc *httpChannel) Close() error {
	return nil
}

func (hc *httpChannel) pull() error {
	if nil == hc.pullurl {
		u, err := url.Parse(hc.addr)
		if nil != err {
			return err
		}
		u.Path = "/http/pull"
		hc.pullurl = u
	}
	if hc.pulling {
		return nil
	}
	readAuth := proxy.NewAuthEvent()
	readAuth.Index = int64(hc.idx)
	readAuth.IV = hc.iv
	var buf bytes.Buffer
	event.EncryptEvent(&buf, readAuth, 0)
	hc.pulling = true
	_, err := hc.postURL(buf.Bytes(), hc.pullurl)
	hc.pulling = false
	return err
}

func (hc *httpChannel) Read(p []byte) (int, error) {
	if hc.rbody == nil && hc.idx >= 0 {
		hc.pull()
	}
	start := time.Now()
	for nil == hc.rbody {
		if time.Now().After(start.Add(5 * time.Second)) {
			return 0, proxy.ErrChannelReadTimeout
		}
		time.Sleep(1 * time.Millisecond)
	}
	n, err := hc.rbody.Read(p)
	if nil != err {
		hc.rbody.Close()
		hc.rbody = nil
	}
	return n, err
}

func (hc *httpChannel) postURL(p []byte, u *url.URL) (n int, err error) {
	req := &http.Request{
		Method:        "POST",
		URL:           u,
		ProtoMajor:    1,
		ProtoMinor:    1,
		Host:          u.Host,
		Header:        make(http.Header),
		Body:          ioutil.NopCloser(bytes.NewBuffer(p)),
		ContentLength: int64(len(p)),
	}
	req.Close = false
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "image/jpeg")
	if len(proxy.GConf.UserAgent) > 0 {
		req.Header.Set("User-Agent", proxy.GConf.UserAgent)
	}
	response, err := paasHttpClient.Do(req)
	if nil != err || response.StatusCode != 200 { //try once more
		req.Body = ioutil.NopCloser(bytes.NewBuffer(p))
		response, err = paasHttpClient.Do(req)
	}
	if nil != err || response.StatusCode != 200 {
		log.Printf("Failed to write data to PAAS:%s for reason:%v or res:%v", u.String(), err, response)
		return 0, err
	} else {
		if response.ContentLength != 0 && nil != response.Body {
			hc.rbody = response.Body
		}
		return len(p), nil
	}
}

func (hc *httpChannel) Write(p []byte) (n int, err error) {
	if nil == hc.pushurl {
		u, err := url.Parse(hc.addr)
		if nil != err {
			return 0, err
		}
		u.Path = "/http/push"
		hc.pushurl = u
	}
	return hc.postURL(p, hc.pushurl)
}

func newHTTPChannel(addr string, idx int) (*proxy.RemoteChannel, error) {
	rc := &proxy.RemoteChannel{
		Addr:          addr,
		Index:         idx,
		DirectIO:      false,
		OpenJoinAuth:  false,
		WriteJoinAuth: true,
	}

	tc := new(httpChannel)
	tc.addr = addr
	tc.idx = idx
	rc.C = tc

	err := rc.Init()
	if nil != err {
		return nil, err
	}
	return rc, nil
}

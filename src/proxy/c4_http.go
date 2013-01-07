package proxy

import (
	"bytes"
	"event"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var httpTunnelServiceTable = make(map[string]*httpTunnelService)

type pushWorker struct {
	index   int
	server  *url.URL
	working bool
	ch      chan []byte
	cache   *bytes.Buffer
}

func (p *pushWorker) offer(ev event.Event) {
	if p.working {
		event.EncodeEvent(p.cache, ev)
	} else {
       p.working = true 
	}
}

func (p *pushWorker) loop() {
	p.working = true
	writeContent := func(cc []byte) error {
		buf := bytes.NewBuffer(cc)
		p.server.Path = p.server.Path + "push/"
		req := &http.Request{
			Method:        "POST",
			URL:           p.server,
			Host:          p.server.Host,
			Header:        make(http.Header),
			Body:          ioutil.NopCloser(buf),
			ContentLength: int64(buf.Len()),
		}

		req.Header.Set("UserToken", userToken)
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Content-Type", "application/octet-stream")
		if len(c4_cfg.UA) > 0 {
			req.Header.Set("User-Agent", c4_cfg.UA)
		}
		resp, err := c4HttpClient.Do(req)
		if nil != err || resp.StatusCode != 200 {
			return fmt.Errorf("Failed to write content to C4 server:%v", err)
		}
		return nil
	}

	for writeContent(content) != nil {
		time.Sleep(1 * time.Second)
	}
	if nil != p.cache && p.cache.Len() > 0 {
	}
	p.working = false
}

type pullWorker struct {
	index  int
	server *url.URL
}

func (p *pullWorker) loop() {
	data := make([]byte, 8192)
	var buffer bytes.Buffer
	chunkLen := int32(-1)
	for {
		p.server.Path = p.server.Path + "pull/"
		req := &http.Request{
			Method:        "POST",
			URL:           p.server,
			Host:          p.server.Host,
			Header:        make(http.Header),
			Body:          ioutil.NopCloser(buf),
			ContentLength: int64(buf.Len()),
		}

		req.Header.Set("UserToken", userToken)
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Content-Type", "application/octet-stream")
		if len(c4_cfg.UA) > 0 {
			req.Header.Set("User-Agent", c4_cfg.UA)
		}
		resp, err := c4HttpClient.Do(req)

		if nil != err || resp.StatusCode != 200 {
			time.Sleep(1 * time.Second)
		} else {
			resp.Body.Read(data)
		}
		return nil
	}
}

type httpTunnelService struct {
	server *url.URL
	pusher []*pushWorker
	puller []*pullWorker
}

func (serv *httpTunnelService) writeEvent(ev event.Event) {
	index := ev.GetHash() % uint32(len(serv.pusher))
	p := serv.pusher[index]
	p.offer(ev)
}

func httpOfferEvent(server string, ev event.Event) {
	getHttpTunnelService(server).writeEvent(ev)
}

func newHttpTunnelService(server string) *httpTunnelService {
	serv := new(httpTunnelService)
	serv.server, _ = url.Parse(server)
	serv.puller = make([]*pushWorker, c4_cfg.MaxHTTPConn)
	serv.pusher = make([]*pullWorker, c4_cfg.MaxHTTPConn)

	for i, _ := range serv.puller {
		serv.puller[i] = new(pullWorker)
		go serv.puller[i].loop()
	}
	for i, _ := range serv.puller {
		serv.puller[i] = new(pushWorker)
	}
	return serv
}

func getHttpTunnelService(server string) *httpTunnelService {
	serv, exist := httpTunnelServiceTable[server]
	if !exist {
		serv := newHttpTunnelService(server)
	}
	return serv
}

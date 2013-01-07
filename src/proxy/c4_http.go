package proxy

import (
	"bytes"
	"event"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var httpTunnelServiceTable = make(map[string]*httpTunnelService)

type pushWorker struct {
	index   int
	server  *url.URL
	working bool
	cache   bytes.Buffer
	mutex   sync.Mutex
}

func (p *pushWorker) offer(ev event.Event) {
	p.mutex.Lock()
	event.EncodeEvent(&p.cache, ev)
	p.mutex.Unlock()
	go p.tryWriteCache()
}

func (p *pushWorker) tryWriteCache() {
	if p.working || p.cache.Len() == 0 {
		return
	}
	p.mutex.Lock()
	tmp := make([]byte, p.cache.Len())
	p.cache.Read(tmp)
	p.cache.Reset()
	p.mutex.Unlock()
	p.writeContent(tmp)
}

func (p *pushWorker) writeContent(content []byte) {
	p.working = true
	buf := bytes.NewBuffer(content)
	if !strings.HasSuffix(p.server.Path, "push/") {
		p.server.Path = p.server.Path + "push/"
	}
	req := &http.Request{
		Method:        "POST",
		URL:           p.server,
		Host:          p.server.Host,
		Header:        make(http.Header),
		Body:          ioutil.NopCloser(buf),
		ContentLength: int64(buf.Len()),
	}

	req.Header.Set("UserToken", userToken)
	req.Header.Set("C4MiscInfo", fmt.Sprintf("%d_%d", p.index, c4_cfg.ReadTimeout))
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/octet-stream")
	if len(c4_cfg.UA) > 0 {
		req.Header.Set("User-Agent", c4_cfg.UA)
	}
	resp, err := c4HttpClient.Do(req)
	if nil != err || resp.StatusCode != 200 {
		log.Printf("Push worker recevice error:%v\n", err)
		time.Sleep(1 * time.Second)
		p.writeContent(content)
	}
	p.working = false
	p.tryWriteCache()
}

type pullWorker struct {
	index  int
	server *url.URL
}

func (p *pullWorker) loop() {
	cumulate := new(C4CumulateTask)
	if !strings.HasSuffix(p.server.Path, "pull/") {
		p.server.Path = p.server.Path + "pull/"
	}
	for {
		req := &http.Request{
			Method:        "POST",
			URL:           p.server,
			Host:          p.server.Host,
			Header:        make(http.Header),
			ContentLength: 0,
		}

		req.Header.Set("UserToken", userToken)
		req.Header.Set("C4MiscInfo", fmt.Sprintf("%d_%d", p.index, c4_cfg.ReadTimeout))
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Content-Type", "application/octet-stream")
		if len(c4_cfg.UA) > 0 {
			req.Header.Set("User-Agent", c4_cfg.UA)
		}
		resp, err := c4HttpClient.Do(req)

		if nil != err || resp.StatusCode != 200 {
			time.Sleep(1 * time.Second)
		} else {
			cumulate.fillContent(resp.Body)
		}
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
	u, err := url.Parse(server)
	if nil != err {

	}
	serv := new(httpTunnelService)
	serv.server, _ = url.Parse(server)
	serv.puller = make([]*pullWorker, c4_cfg.MaxConn)
	serv.pusher = make([]*pushWorker, c4_cfg.MaxConn)

	for i, _ := range serv.puller {
		serv.puller[i] = new(pullWorker)
		serv.puller[i].index = i
		serv.puller[i].server = u
		go serv.puller[i].loop()
	}
	for i, _ := range serv.puller {
		serv.pusher[i] = new(pushWorker)
		serv.pusher[i].index = i
		serv.pusher[i].server = u
	}
	return serv
}

func getHttpTunnelService(server string) *httpTunnelService {
	serv, exist := httpTunnelServiceTable[server]
	if !exist {
		serv = newHttpTunnelService(server)
	}
	return serv
}

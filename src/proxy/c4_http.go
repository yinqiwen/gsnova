package proxy

import (
	"bytes"
	"common"
	"crypto/rc4"
	"encoding/base64"
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
	if !strings.HasSuffix(p.server.Path, "push") {
		p.server.Path = p.server.Path + "push"
	}
	req := &http.Request{
		Method:        "POST",
		URL:           p.server,
		Host:          p.server.Host,
		Header:        make(http.Header),
		Body:          ioutil.NopCloser(buf),
		ContentLength: int64(buf.Len()),
	}
	if c4_cfg.Encrypter == event.ENCRYPTER_RC4 {
		tmp := []byte(common.RC4Key)
		cipher, _ := rc4.NewCipher(tmp)
		dst := make([]byte, len(tmp))
		cipher.XORKeyStream(dst, tmp)
		req.Header.Set("RC4Key", base64.StdEncoding.EncodeToString(dst))
	}

	req.Header.Set("UserToken", userToken)
	req.Header.Set("C4MiscInfo", fmt.Sprintf("%d_%d", p.index, c4_cfg.ReadTimeout))
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/octet-stream")
	if len(c4_cfg.UA) > 0 {
		req.Header.Set("User-Agent", c4_cfg.UA)
	}
	resp, err := c4HttpClient.Do(req)
	fail := false
	if nil != err {
		fail = true
		log.Printf("Push worker recevice error:%v  %v\n", err, p.server)
	}
	if nil == err && resp.StatusCode != 200 {
		fail = true
		log.Printf("Push worker recevice error response :%v\n", resp)
	}
	if fail {
		if nil != resp {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
		p.writeContent(content)
	}
	if nil != resp {
		resp.Body.Close()
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
	cumulate.chunkLen = -1
	if !strings.HasSuffix(p.server.Path, "pull") {
		p.server.Path = p.server.Path + "pull"
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
		log.Printf("Pull worker:%d start working\n", p.index)
		resp, err := c4HttpClient.Do(req)

		if nil != err || resp.StatusCode != 200 {
			log.Printf("Pull worker:%d recv invalid res:%v\n", p.index, resp)
			time.Sleep(1 * time.Second)
		} else {
			//log.Printf("Got chunked %v %v\n", resp.TransferEncoding, resp.Header)
			cumulate.fillContent(resp.Body)
		}
		if nil != resp && nil != resp.Body {
			resp.Body.Close()
		}
		log.Printf("Pull worker:%d stop working\n", p.index)
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
		u, _ = url.Parse(server)
		serv.puller[i].server = u
		go serv.puller[i].loop()
	}
	for i, _ := range serv.puller {
		serv.pusher[i] = new(pushWorker)
		serv.pusher[i].index = i
		u, _ = url.Parse(server)
		serv.pusher[i].server = u
	}
	return serv
}

func getHttpTunnelService(server string) *httpTunnelService {
	serv, exist := httpTunnelServiceTable[server]
	if !exist {
		serv = newHttpTunnelService(server)
		httpTunnelServiceTable[server] = serv
	}
	return serv
}

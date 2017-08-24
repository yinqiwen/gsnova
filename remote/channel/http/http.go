package http

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/remote"
	"github.com/yinqiwen/pmux"
)

type httpDuplexServConn struct {
	id           string
	recvBuffer   bytes.Buffer
	writer       http.ResponseWriter
	recvLock     sync.Mutex
	sendLock     sync.Mutex
	running      bool
	recvNotifyCh chan struct{}
	sendNotifyCh chan struct{}
}

func (h *httpDuplexServConn) setReader(req *http.Request) {
	b := make([]byte, 8192)
	for {
		n, err := req.Body.Read(b)
		if n > 0 {
			h.recvLock.Lock()
			h.recvBuffer.Write(b[0:n])
			h.recvLock.Unlock()
			helper.AsyncNotify(h.recvNotifyCh)
		}
		if nil != err {
			break
		}
	}
}

func (h *httpDuplexServConn) setWriter(w http.ResponseWriter) {
	h.sendLock.Lock()
	h.writer = w
	h.sendLock.Unlock()
	helper.AsyncNotify(h.sendNotifyCh)
}

func (h *httpDuplexServConn) init(id string) error {
	h.id = id
	h.recvNotifyCh = make(chan struct{})
	h.sendNotifyCh = make(chan struct{})
	h.running = true
	return nil
}

func (h *httpDuplexServConn) Read(b []byte) (n int, err error) {
START:
	if !h.running {
		return 0, io.EOF
	}
	h.recvLock.Lock()
	if 0 == h.recvBuffer.Len() {
		h.recvLock.Unlock()
		goto WAIT
	}
	n, _ = h.recvBuffer.Read(b)
	h.recvLock.Unlock()
	return n, nil
WAIT:
	var timeout <-chan time.Time
	var timer *time.Timer
	timer = time.NewTimer(time.Duration(10) * time.Second)
	timeout = timer.C
	select {
	case <-h.recvNotifyCh:
		if timer != nil {
			timer.Stop()
		}
		goto START
	case <-timeout:
		goto START
	}
}

func (h *httpDuplexServConn) Write(p []byte) (n int, err error) {
START:
	if !h.running {
		return 0, io.EOF
	}
	h.sendLock.Lock()
	if nil == h.writer {
		h.sendLock.Unlock()
		goto WAIT
	}
	n, err = h.writer.Write(p)
	if nil == err {
		h.writer.(http.Flusher).Flush()
	} else {
		h.writer = nil
	}
	h.sendLock.Unlock()
	return n, err
WAIT:
	var timeout <-chan time.Time
	var timer *time.Timer
	timer = time.NewTimer(time.Duration(10) * time.Second)
	timeout = timer.C
	select {
	case <-h.sendNotifyCh:
		if timer != nil {
			timer.Stop()
		}
		goto START
	case <-timeout:
		goto START
	}
}

func (h *httpDuplexServConn) closeWrite() error {
	if h.running {
		h.sendLock.Lock()
		h.writer = nil
		h.sendLock.Unlock()
		helper.AsyncNotify(h.sendNotifyCh)
	}
	return nil
}

func (h *httpDuplexServConn) Close() error {
	if h.running {
		h.running = false
		h.sendLock.Lock()
		h.writer = nil
		h.sendLock.Unlock()
		helper.AsyncNotify(h.recvNotifyCh)
		helper.AsyncNotify(h.sendNotifyCh)
	}
	removetHttpDuplexServConnByID(h.id)
	return nil
}

var httpDuplexServConnTable = make(map[string]*httpDuplexServConn)
var httpDuplexServConnMutex sync.Mutex

func getHttpDuplexServConnByID(id string, createIfNotExist bool) (*httpDuplexServConn, bool) {
	httpDuplexServConnMutex.Lock()
	defer httpDuplexServConnMutex.Unlock()

	c, exist := httpDuplexServConnTable[id]
	if !exist {
		if createIfNotExist {
			c = &httpDuplexServConn{}
			c.init(id)
			httpDuplexServConnTable[id] = c
			return c, true
		}
	}
	return c, false
}

func removetHttpDuplexServConnByID(id string) {
	httpDuplexServConnMutex.Lock()
	defer httpDuplexServConnMutex.Unlock()
	delete(httpDuplexServConnTable, id)
}

func httpTest(w http.ResponseWriter, r *http.Request) {
	log.Printf("###Test req:%v", r)
	w.Write([]byte("OK"))
}

func HTTPInvoke(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("X-Session-ID")
	if len(id) == 0 {
		log.Printf("Invalid header with no session id:%v", r)
		return
	}
	c, create := getHttpDuplexServConnByID(id, true)
	if create {
		//c.setReader()
		session, err := pmux.Server(c, remote.InitialPMuxConfig())
		if nil != err {
			return
		}
		muxSession := &mux.ProxyMuxSession{Session: session}
		go remote.ServProxyMuxSession(muxSession)
	}
	if strings.HasSuffix(r.URL.Path, "pull") {
		c.setWriter(w)
		w.WriteHeader(200)
		period, _ := strconv.Atoi(r.Header.Get("X-PullPeriod"))
		if period <= 0 {
			period = 30
		}
		select {
		case <-time.After(time.Duration(period) * time.Second):
			c.closeWrite()
			log.Printf("HTTP server close pull for id:%s", id)
			return
		}
	} else {
		c.setReader(r)
	}
}

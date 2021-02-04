package http

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/pmux"
)

type httpDuplexServConn struct {
	id               string
	ackID            string
	recvBuffer       bytes.Buffer
	req              *http.Request
	writer           http.ResponseWriter
	writerStopCh     chan struct{}
	recvLock         sync.Mutex
	sendLock         sync.Mutex
	running          int32
	recvNotifyCh     chan struct{}
	sendNotifyCh     chan struct{}
	closeNotifyCh    chan struct{}
	shutdownErr      error
	lastActiveIOTime time.Time
	checkAliveTicker *time.Ticker
}

func (h *httpDuplexServConn) touch() {
	h.lastActiveIOTime = time.Now()
}

func (h *httpDuplexServConn) setReader(req *http.Request) {
	h.touch()
	h.req = req
	b := make([]byte, 8192)
	counter := 0
	for {
		n, err := req.Body.Read(b)
		if n > 0 {
			h.recvLock.Lock()
			h.recvBuffer.Write(b[0:n])
			h.recvLock.Unlock()
			h.touch()
			helper.AsyncNotify(h.recvNotifyCh)
		}
		counter += n
		if nil != err {
			break
		}
	}
	//log.Printf("#####Chunk read %d bytes", counter)
	h.req = nil
}

func (h *httpDuplexServConn) setWriter(w http.ResponseWriter, ch chan struct{}) {
	h.touch()
	h.sendLock.Lock()
	if nil != h.writerStopCh {
		helper.AsyncNotify(h.writerStopCh)
	}
	h.writer = w
	h.writerStopCh = ch
	h.sendLock.Unlock()
	helper.AsyncNotify(h.sendNotifyCh)
}

func (h *httpDuplexServConn) init(id string) error {
	h.id = id
	h.ackID = helper.RandAsciiString(32)
	h.recvNotifyCh = make(chan struct{})
	h.sendNotifyCh = make(chan struct{})
	h.closeNotifyCh = make(chan struct{})
	h.checkAliveTicker = time.NewTicker(10 * time.Second)
	h.lastActiveIOTime = time.Now()
	go func() {
		for _ = range h.checkAliveTicker.C {
			if !h.isRunning() {
				h.checkAliveTicker.Stop()
				return
			}
			if time.Now().Sub(h.lastActiveIOTime) > 2*time.Minute {
				h.checkAliveTicker.Stop()
				h.Close()
				logger.Debug("Stop http duplex conn:%s since it's not active since %v ago", h.id, time.Now().Sub(h.lastActiveIOTime))
				return
			}
		}
	}()
	h.running = 1
	return nil
}

func (h *httpDuplexServConn) Read(b []byte) (n int, err error) {
START:
	if !h.isRunning() {
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
	if !h.isRunning() {
		return 0, io.EOF
	}
	h.sendLock.Lock()
	if nil == h.writer {
		h.sendLock.Unlock()
		goto WAIT
	}
	h.touch()
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
	if h.isRunning() {
		h.sendLock.Lock()
		h.writer = nil
		if nil != h.writerStopCh {
			helper.AsyncNotify(h.writerStopCh)
		}
		h.sendLock.Unlock()
		helper.AsyncNotify(h.sendNotifyCh)
	}
	return nil
}

func (h *httpDuplexServConn) closeRead() error {
	req := h.req
	if nil != req && nil != req.Body {
		req.Body.Close()
	}
	return nil
}

func (h *httpDuplexServConn) shutdown(err error) {
	h.shutdownErr = err
	h.Close()
	h.closeRead()
}

func (h *httpDuplexServConn) isRunning() bool {
	return atomic.LoadInt32(&h.running) > 0
}

func (h *httpDuplexServConn) Close() error {
	if h.isRunning() {
		h.closeWrite()
		helper.AsyncNotify(h.recvNotifyCh)
		helper.AsyncNotify(h.closeNotifyCh)
		atomic.StoreInt32(&h.running, 0)
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

func HttpTest(w http.ResponseWriter, r *http.Request) {
	//log.Printf("###Test req:%v", r)
	w.Write([]byte("OK"))
}

func HTTPInvoke(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get(mux.HTTPMuxSessionIDHeader)
	if len(id) == 0 {
		logger.Debug("Invalid header with no session id:%v", r)
		return
	}
	c, create := getHttpDuplexServConnByID(id, true)
	if create {
		if len(r.Header.Get(mux.HTTPMuxSessionACKIDHeader)) > 0 {
			w.WriteHeader(401)
			logger.Error("###ERR1 : %s", r.Header.Get(mux.HTTPMuxSessionACKIDHeader))
			return
		}
		session, err := pmux.Server(c, channel.InitialPMuxConfig(&channel.DefaultServerCipher))
		if nil != err {
			return
		}
		muxSession := &mux.ProxyMuxSession{Session: session}
		go func() {
			err := channel.ServProxyMuxSession(muxSession, nil, nil)
			if nil != err {
				c.shutdown(err)
			}
		}()
	}
	ackID := r.Header.Get(mux.HTTPMuxSessionACKIDHeader)
	if len(ackID) > 0 && ackID != c.ackID {
		w.WriteHeader(401)
		logger.Error("###ERR2 : %s %s", r.Header.Get(mux.HTTPMuxSessionACKIDHeader), c.ackID)
		return
	}
	w.Header().Set(mux.HTTPMuxSessionACKIDHeader, c.ackID)
	if strings.HasSuffix(r.URL.Path, "pull") {
		logger.Debug("HTTP server recv pull for id:%s", id)
		period, _ := strconv.Atoi(r.Header.Get("X-PullPeriod"))
		if period <= 0 {
			period = 30
		}
		timer := time.NewTimer(time.Duration(period) * time.Second)
		timeout := timer.C
		stopByOther := make(chan struct{})
		c.setWriter(w, stopByOther)
		if !c.isRunning() {
			w.WriteHeader(401)
			timer.Stop()
			return
		}
		select {
		case <-timeout:
			c.closeWrite()
			logger.Notice("HTTP server close pull for id:%s", id)
			return
		case <-c.closeNotifyCh:
			timer.Stop()
			w.WriteHeader(401)
			logger.Debug("HTTP server close pull for id:%s close ", id)
		case <-stopByOther:
			logger.Debug("HTTP server recv pull id:%s stop by other pull", id)
			timer.Stop()
			return
		}
	} else {
		//counter := r.URL.Query().Get(pmux.HTTPPullCounterKey)
		c.setReader(r)
		if nil != c.shutdownErr {
			w.WriteHeader(401)
		}
	}
}

package http

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/local/proxy"
	"github.com/yinqiwen/pmux"
	"golang.org/x/time/rate"
)

type chunkedBody struct {
	chunkChannel chan []byte
	readBuffer   pmux.ByteSliceBuffer
}

func (cb *chunkedBody) Read(p []byte) (int, error) {
	if cb.readBuffer.Len() == 0 {
		b := <-cb.chunkChannel
		if len(b) == 0 {
			return 0, io.EOF
		}
		cb.readBuffer.Write(b)
	}
	n, _ := cb.readBuffer.Read(p)
	return n, nil
}
func (cb *chunkedBody) Close() error {
	return nil
}
func (cr *chunkedBody) offer(p []byte) {
	cr.chunkChannel <- p
}

// sendReady is used to either mark a stream as ready
// or to directly send a header
type sendReady struct {
	Data []byte
	Err  chan error
}

func buildHTTPReq(u *url.URL, body io.ReadCloser) *http.Request {
	req := &http.Request{
		Method:     "POST",
		URL:        u,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Host:       u.Host,
		Header:     make(http.Header),
		Body:       body,
	}
	req.Close = false
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "image/jpeg")
	if len(proxy.GConf.UserAgent) > 0 {
		req.Header.Set("User-Agent", proxy.GConf.UserAgent)
	}
	return req
}

type httpDuplexConn struct {
	id           string
	server       string
	conf         *proxy.ProxyChannelConfig
	client       *http.Client
	pushurl      *url.URL
	pullurl      *url.URL
	testurl      *url.URL
	recvReader   io.ReadCloser
	recvLock     sync.Mutex
	running      bool
	sendCh       chan sendReady
	closeCh      chan struct{}
	recvNotifyCh chan struct{}
	pullNotifyCh chan struct{}

	pushLimiter *rate.Limiter

	chunkPushBody       chunkedBody
	chunkPushSupported  bool
	chunkPushExpireTime time.Time
}

func (h *httpDuplexConn) testChunkPush() {
	var empty bytes.Buffer
	req := buildHTTPReq(h.testurl, ioutil.NopCloser(&empty))
	req.ContentLength = -1
	response, err := h.client.Do(req)
	if nil != err || response.StatusCode != 200 {
		h.chunkPushSupported = false
		return
	}
	if nil != response.Body {
		response.Body.Close()
	}
	h.chunkPushSupported = true
	log.Printf("Server:%s support chunked transfer encoding request.", h.server)
}

func (h *httpDuplexConn) init(server string) error {
	h.server = server
	_, err := url.Parse(server)
	if nil != err {
		return err
	}
	h.id = helper.RandAsciiString(64)
	h.pushurl, _ = url.Parse(server + "/http/push")
	h.pullurl, _ = url.Parse(server + "/http/pull")
	h.testurl, _ = url.Parse(server + "/http/test")
	h.testChunkPush()
	h.sendCh = make(chan sendReady, 10)
	h.closeCh = make(chan struct{})
	h.recvNotifyCh = make(chan struct{})
	h.pullNotifyCh = make(chan struct{})
	h.chunkPushBody.chunkChannel = make(chan []byte)
	h.pushLimiter = rate.NewLimiter(5, 1)
	h.running = true
	if h.chunkPushSupported {
		go h.chunkPush()
	}
	go h.push()
	go h.pull()
	return nil
}

func (h *httpDuplexConn) chunkPush() {
	for h.running {
		h.pushLimiter.Wait(context.TODO())
		log.Printf("HTTP start chunked push for %v with id:%s", h.pushurl, h.id)
		req := buildHTTPReq(h.pushurl, &h.chunkPushBody)
		req.Header.Add("X-Session-ID", h.id)
		req.ContentLength = -1
		h.chunkPushExpireTime = time.Now().Add(time.Duration(h.conf.ReconnectPeriod) * time.Second)
		h.client.Do(req)
	}
}

func (h *httpDuplexConn) pull() {
	for h.running {
		if nil != h.recvReader {
			select {
			case <-h.pullNotifyCh:
			case <-time.After(10 * time.Second):
			}
			continue
		}
		log.Printf("HTTP start pull for %v with id:%s", h.pullurl, h.id)
		req := buildHTTPReq(h.pullurl, nil)
		req.Header.Add("X-Session-ID", h.id)
		req.Header.Set("X-PullPeriod", strconv.Itoa(h.conf.ReconnectPeriod))
		//period := hc.getHTTPReconnectPeriod()
		//req.Header.Set("X-PullPeriod", strconv.Itoa(period))
		response, err := h.client.Do(req)
		if nil != err || response.StatusCode != 200 { //try once more
			log.Printf("Failed to write data to HTTP server for reason:%v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		h.recvLock.Lock()
		h.recvReader = response.Body
		h.recvLock.Unlock()
		helper.AsyncNotify(h.recvNotifyCh)
	}
}

func (h *httpDuplexConn) push() {
	readFrames := func() ([]sendReady, error) {
		var frs []sendReady
		for len(h.sendCh) > 0 {
			frame := <-h.sendCh
			frs = append(frs, frame)
		}
		if len(frs) == 0 {
			select {
			case frame := <-h.sendCh:
				frs = append(frs, frame)
			case <-h.closeCh:
				return frs, helper.ErrConnReset
			case <-time.After(5 * time.Second):
			}
		}
		return frs, nil
	}
	var frs []sendReady

	notifySuccess := func() {
		for _, frame := range frs {
			helper.AsyncSendErr(frame.Err, nil)
		}
		frs = make([]sendReady, 0)
	}
	var err error
	for h.running {
		sendBuffer := &bytes.Buffer{}
		if len(frs) == 0 {
			frs, err = readFrames()
			if nil != err {
				log.Printf("[ERR] pmux: Failed to write frames: %v", err)
				break
			}
		}
		for _, frame := range frs {
			//log.Printf("###Write %d", frame.Header.Flags())
			sendBuffer.Write(frame.Data)
		}

		if h.chunkPushSupported {
			if !h.chunkPushExpireTime.IsZero() && h.chunkPushExpireTime.Before(time.Now()) {
				h.chunkPushExpireTime = time.Time{}
				h.chunkPushBody.offer(nil)
			}
			if sendBuffer.Len() > 0 {
				h.chunkPushBody.offer(sendBuffer.Bytes())
			}
			notifySuccess()
		} else {
			if sendBuffer.Len() > 0 {
				req := buildHTTPReq(h.pushurl, ioutil.NopCloser(sendBuffer))
				req.Header.Add("X-Session-ID", h.id)
				response, err := h.client.Do(req)
				if nil != err || response.StatusCode != 200 { //try once more
					log.Printf("Failed to write data to HTTP server:%s for reason:%v or res:%v", h.pushurl.String(), err, response)
				} else {
					notifySuccess()
				}
				if nil != response && response.Body != nil {
					response.Body.Close()
				}
			}
		}
	}
	for _, frame := range frs {
		helper.AsyncSendErr(frame.Err, err)
	}
}

func (h *httpDuplexConn) Read(b []byte) (n int, err error) {
START:
	h.recvLock.Lock()
	if nil == h.recvReader {
		h.recvLock.Unlock()
		if !h.running {
			return 0, io.EOF
		}
		goto WAIT
	}
	n, err = h.recvReader.Read(b)
	if nil != err {
		h.recvReader.Close()
		h.recvReader = nil
		helper.AsyncNotify(h.pullNotifyCh)
	}
	h.recvLock.Unlock()
	return n, nil
WAIT:
	select {
	case <-h.recvNotifyCh:
		goto START
	case <-time.After(10 * time.Second):
		goto START
	}
}

func (h *httpDuplexConn) Write(p []byte) (int, error) {
	ready := sendReady{Data: p, Err: make(chan error, 1)}
START:
	if !h.running {
		return 0, io.EOF
	}

	select {
	case h.sendCh <- ready:
	case <-h.closeCh:
		return 0, io.EOF
	case <-time.After(5 * time.Second):
		goto START
	}

	select {
	case err := <-ready.Err:
		return len(p), err
	case <-h.closeCh:
		return 0, io.EOF
	case <-time.After(5 * time.Second):
		goto START
	}
}
func (h *httpDuplexConn) Close() error {
	if h.running {
		h.running = false
		close(h.closeCh)
		close(h.chunkPushBody.chunkChannel)
	}
	return nil
}

type HTTPProxy struct {
	//proxy.BaseProxy
}

func (ws *HTTPProxy) CreateMuxSession(server string, conf *proxy.ProxyChannelConfig) (mux.MuxSession, error) {
	conn := &httpDuplexConn{}
	conn.conf = conf
	conn.client, _ = proxy.NewHTTPClient(conf)
	err := conn.init(server)
	if nil != err {
		return nil, err
	}
	log.Printf("Connect %s success.", server)
	ps, err := pmux.Client(conn, proxy.InitialPMuxConfig())
	if nil != err {
		return nil, err
	}
	return &mux.ProxyMuxSession{Session: ps}, nil
}

func init() {
	proxy.RegisterProxyType("http", &HTTPProxy{})
	proxy.RegisterProxyType("https", &HTTPProxy{})
}

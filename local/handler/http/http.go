package http11

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/local/proxy"
	"github.com/yinqiwen/pmux"
)

var errHTTPDuplexConnReset = fmt.Errorf("HTTP duplex conn reset")

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
	conf         proxy.ProxyChannelConfig
	client       *http.Client
	pushurl      *url.URL
	pullurl      *url.URL
	recvbuf      bytes.Buffer
	recvLock     sync.Mutex
	running      bool
	sendCh       chan sendReady
	closeCh      chan struct{}
	recvNotifyCh chan struct{}
}

func (h *httpDuplexConn) init(server string) error {
	_, err := url.Parse(server)
	if nil != err {
		return err
	}
	id := helper.RandAsciiString(64)
	h.pushurl, _ = url.Parse(server + "/http/push/" + id)
	h.pullurl, _ = url.Parse(server + "/http/pull/" + id)
	h.sendCh = make(chan sendReady, 10)
	h.closeCh = make(chan struct{})
	h.recvNotifyCh = make(chan struct{})
	h.running = true
	go h.push()
	go h.pull()
	return nil
}

func (h *httpDuplexConn) pull() {
	buf := make([]byte, 8192)
	for h.running {
		log.Printf("HTTP11 start pulling for %v", h.pullurl)
		req := buildHTTPReq(h.pullurl, nil)
		//period := hc.getHTTPReconnectPeriod()
		//req.Header.Set("X-PullPeriod", strconv.Itoa(period))
		response, err := h.client.Do(req)
		if nil != err || response.StatusCode != 200 { //try once more
			log.Printf("Failed to write data to HTTP1.1 server:%s for reason:%v or res:%v", h.pullurl.String(), err, response)
			time.Sleep(1 * time.Second)
			continue
		}
		for {
			n, err := response.Body.Read(buf)
			if n > 0 {
				h.recvLock.Lock()
				h.recvbuf.Write(buf[0:n])
				h.recvLock.Unlock()
				//notify reader
				helper.AsyncNotify(h.recvNotifyCh)
			}
			if nil != err {
				break
			}
		}
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
				return frs, errHTTPDuplexConnReset
			}
		}
		return frs, nil
	}
	var frs []sendReady
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
		req := buildHTTPReq(h.pushurl, ioutil.NopCloser(sendBuffer))
		//req.Header.Set("X-PullPeriod", strconv.Itoa(period))
		response, err := h.client.Do(req)
		if nil != err || response.StatusCode != 200 { //try once more
			log.Printf("Failed to write data to HTTP server:%s for reason:%v or res:%v", h.pushurl.String(), err, response)
		} else {
			for _, frame := range frs {
				helper.AsyncSendErr(frame.Err, nil)
			}
			frs = make([]sendReady, 0)
		}
		if response.Body != nil {
			response.Body.Close()
		}
	}
	for _, frame := range frs {
		helper.AsyncSendErr(frame.Err, err)
	}
}

func (h *httpDuplexConn) Read(b []byte) (n int, err error) {
START:
	h.recvLock.Lock()
	if h.recvbuf.Len() == 0 {
		h.recvLock.Unlock()
		goto WAIT
	}

	// Read any bytes
	n, _ = h.recvbuf.Read(b)
	return n, nil
WAIT:
	var timeout <-chan time.Time
	var timer *time.Timer
	if h.conf.ReadTimeout > 0 {
		//delay := s.readDeadline.Sub(time.Now())
		timer = time.NewTimer(time.Duration(h.conf.ReadTimeout) * time.Second)
		timeout = timer.C
	}
	select {
	case <-h.recvNotifyCh:
		if timer != nil {
			timer.Stop()
		}
		goto START
	case <-timeout:
		return 0, helper.ErrReadTimeout
	}
}

func (h *httpDuplexConn) Write(p []byte) (int, error) {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	ready := sendReady{Data: p, Err: make(chan error, 1)}
	select {
	case h.sendCh <- ready:
	case <-h.closeCh:
		return 0, errHTTPDuplexConnReset
	case <-timer.C:
		return 0, helper.ErrWriteTimeout
	}

	select {
	case err := <-ready.Err:
		return len(p), err
	case <-h.closeCh:
		return 0, errHTTPDuplexConnReset
	case <-timer.C:
		return 0, helper.ErrWriteTimeout
	}
}
func (h *httpDuplexConn) Close() error {
	if h.running {
		h.running = false
		close(h.closeCh)
	}
	return nil
}

type HTTPProxy struct {
	proxy.BaseProxy
}

func (ws *HTTPProxy) CreateMuxSession(server string) (proxy.MuxSession, error) {
	conn := &httpDuplexConn{}
	err := conn.init(server)
	if nil != err {
		return nil, err
	}
	log.Printf("Connect %s success.", server)
	ps, err := pmux.Client(conn, nil)
	if nil != err {
		return nil, err
	}
	return &proxy.ProxyMuxSession{Session: ps}, nil
}

func init() {
	proxy.RegisterProxyType("HTTP", &HTTPProxy{})
}

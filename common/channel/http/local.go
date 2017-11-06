package http

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/pmux"
	"golang.org/x/time/rate"
)

type chunkedBody struct {
	chunkChannel chan []byte
	readBuffer   pmux.ByteSliceBuffer
	counter      int
}

func (cb *chunkedBody) Read(p []byte) (int, error) {
	if cb.readBuffer.Len() == 0 {
		b := <-cb.chunkChannel
		if nil == b {
			return 0, io.EOF
		}
		cb.readBuffer.Write(b)
	}
	n, _ := cb.readBuffer.Read(p)
	cb.counter += n
	return n, nil
}
func (cb *chunkedBody) Close() error {
	select {
	case cb.chunkChannel <- nil:
	default:
		return nil
	}
	return nil
}
func (cr *chunkedBody) offer(p []byte) error {
	select {
	case cr.chunkChannel <- p:
		return nil
	case <-time.After(100 * time.Millisecond):
		return pmux.ErrTimeout
	}
}

func newChunkedBody(buffer pmux.ByteSliceBuffer) *chunkedBody {
	cr := new(chunkedBody)
	cr.chunkChannel = make(chan []byte)
	cr.readBuffer = buffer
	return cr
}

// sendReady is used to either mark a stream as ready
// or to directly send a header
type sendReady struct {
	Data []byte
	Err  chan error
}

type httpDuplexConn struct {
	id           string
	ackID        string
	server       string
	conf         *channel.ProxyChannelConfig
	client       *http.Client
	pushurl      *url.URL
	pullurl      *url.URL
	testurl      *url.URL
	recvReader   io.ReadCloser
	recvLock     sync.Mutex
	writeLock    sync.Mutex
	running      bool
	sendCh       chan sendReady
	closeCh      chan struct{}
	recvNotifyCh chan struct{}
	pullNotifyCh chan struct{}

	pushLimiter *rate.Limiter

	chunkPushBody      *chunkedBody
	chunkPushSupported bool
}

func (h *httpDuplexConn) buildHTTPReq(u *url.URL, body io.ReadCloser) *http.Request {
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
	if len(h.conf.HTTP.UserAgent) > 0 {
		req.Header.Set("User-Agent", h.conf.HTTP.UserAgent)
	}
	req.Header.Set(mux.HTTPMuxSessionIDHeader, h.id)
	if len(h.ackID) > 0 {
		req.Header.Set(mux.HTTPMuxSessionACKIDHeader, h.ackID)
	}
	return req
}

func (h *httpDuplexConn) testChunkPush() {
	var empty bytes.Buffer
	req := h.buildHTTPReq(h.testurl, ioutil.NopCloser(&empty))
	req.ContentLength = -1
	response, err := h.client.Do(req)
	if nil != err || response.StatusCode != 200 {
		h.chunkPushSupported = false
		logger.Notice("Server:%s do NOT support chunked transfer encoding request.", h.server)
		return
	}
	if nil != response.Body {
		response.Body.Close()
	}
	h.chunkPushSupported = true
	h.newChunkPushBody()
	logger.Notice("Server:%s support chunked transfer encoding request.", h.server)
}

func (h *httpDuplexConn) init(server string, pushRateLimit int) error {
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
	h.pushLimiter = rate.NewLimiter(rate.Limit(pushRateLimit), 1)
	h.running = true
	if h.chunkPushSupported {
		go h.chunkPush()
	}
	go h.push()
	go h.pull()
	return nil
}

func (h *httpDuplexConn) setAckId(res *http.Response) {
	if nil != res && res.StatusCode == 200 {
		if len(h.ackID) == 0 {
			h.ackID = res.Header.Get(mux.HTTPMuxSessionACKIDHeader)
		}
	}
}

func (h *httpDuplexConn) newChunkPushBody() {
	h.writeLock.Lock()
	if nil == h.chunkPushBody {
		h.chunkPushBody = newChunkedBody(nil)
	} else {
		h.chunkPushBody = newChunkedBody(h.chunkPushBody.readBuffer)
	}
	h.writeLock.Unlock()
}

func (h *httpDuplexConn) chunkPush() {
	var restartChunkPushTimer *time.Timer

	for h.running {
		h.pushLimiter.Wait(context.TODO())
		logger.Debug("HTTP start chunked push for %v with id:%s", h.pushurl, h.id)
		req := h.buildHTTPReq(h.pushurl, h.chunkPushBody)
		req.ContentLength = -1
		restartChunkPushTimer = time.NewTimer(time.Duration(h.conf.ReconnectPeriod) * time.Second)

		go func() {
			select {
			case <-restartChunkPushTimer.C:
				h.writeLock.Lock()
				h.chunkPushBody.Close()
				h.writeLock.Unlock()
			}
		}()
		res, err := h.client.Do(req)
		if nil != res && res.StatusCode == 401 {
			logger.Notice("Failed to chunk push to HTTP server for response:%v", res)
			h.Close()
			return
		}
		h.newChunkPushBody()
		restartChunkPushTimer.Stop()
		h.setAckId(res)
		if nil != err {
			time.Sleep(1 * time.Second)
		}
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
		req := h.buildHTTPReq(h.pullurl, nil)
		req.Header.Set(mux.HTTPMuxPullPeriodHeader, strconv.Itoa(h.conf.ReconnectPeriod))
		response, err := h.client.Do(req)
		if nil != err {
			logger.Notice("Failed to write data to HTTP server for reason:%v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		if response.StatusCode == 401 { //try once more
			logger.Notice("Failed to pull data from HTTP server for response:%v ", response)
			h.Close()
			return
		}
		if response.StatusCode != 200 {
			time.Sleep(1 * time.Second)
			continue
		}
		h.setAckId(response)
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
	var err error
	notifyDone := func() {
		for _, frame := range frs {
			helper.AsyncSendErr(frame.Err, err)
		}
		frs = make([]sendReady, 0)
	}
	for h.running {
		sendBuffer := &bytes.Buffer{}
		if len(frs) == 0 {
			frs, err = readFrames()
			if nil != err {
				logger.Notice("[ERR] Failed to read frames: %v", err)
				break
			}
		}
		for _, frame := range frs {
			//log.Printf("###Write %d", frame.Header.Flags())
			sendBuffer.Write(frame.Data)
		}

		if h.chunkPushSupported {
			if sendBuffer.Len() > 0 {
				h.writeLock.Lock()
				err = h.chunkPushBody.offer(sendBuffer.Bytes())
				h.writeLock.Unlock()
				if nil != err {
					continue
				}
				notifyDone()
			}
		} else {
			if sendBuffer.Len() > 0 {
				req := h.buildHTTPReq(h.pushurl, ioutil.NopCloser(sendBuffer))
				req.ContentLength = int64(sendBuffer.Len())
				response, err := h.client.Do(req)
				h.setAckId(response)
				if nil != err || response.StatusCode != 200 { //try once more
					logger.Notice("Failed to write data to HTTP server:%s for reason:%v or res:%v", h.pushurl.String(), err, response)
					if nil != response && response.StatusCode == 401 {
						h.Close()
						return
					}
					time.Sleep(1 * time.Second)
				} else {
					err = nil
					notifyDone()
				}
				if nil != response && response.Body != nil {
					response.Body.Close()
				}
			}
		}
	}
	notifyDone()
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
		if nil != h.chunkPushBody {
			h.chunkPushBody.Close()
		}
	}
	return nil
}

type HTTPProxy struct {
	//proxy.BaseProxy
}

func (p *HTTPProxy) Features() channel.FeatureSet {
	return channel.FeatureSet{
		AutoExpire: false,
		Pingable:   true,
	}
}

func (ws *HTTPProxy) CreateMuxSession(server string, conf *channel.ProxyChannelConfig) (mux.MuxSession, error) {
	conn := &httpDuplexConn{}
	conn.conf = conf
	u, _ := url.Parse(server)
	conn.client, _ = channel.NewHTTPClient(conf, u.Scheme)
	err := conn.init(server, conf.HTTP.HTTPPushRateLimitPerSec)
	if nil != err {
		return nil, err
	}
	//log.Printf("Connect %s success.", server)
	ps, err := pmux.Client(conn, channel.InitialPMuxConfig(&conf.Cipher))
	if nil != err {
		return nil, err
	}
	return &mux.ProxyMuxSession{Session: ps}, nil
}

func init() {
	channel.RegisterLocalChannelType("http", &HTTPProxy{})
	channel.RegisterLocalChannelType("https", &HTTPProxy{})
}

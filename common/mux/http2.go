package mux

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/pmux"
	"golang.org/x/net/http2"
)

// type cachedClientConnPool struct {
// 	conns sync.Map
// }

// func (c *cachedClientConnPool) GetClientConn(req *http.Request, addr string) (*http2.ClientConn, error) {
// 	var conn *http2.ClientConn
// 	c.conns.Range(func(key, value interface{}) bool {
// 		conn = key.(*http2.ClientConn)
// 		return false
// 	})
// 	if nil != conn {
// 		return conn, nil
// 	}
// 	return nil, fmt.Errorf("Existing connection closed")
// }
// func (c *cachedClientConnPool) MarkDead(conn *http2.ClientConn) {
// 	c.conns.Delete(conn)
// }

// func (c *cachedClientConnPool) add(conn *http2.ClientConn) {
// 	c.conns.Store(conn, true)
// }

// var cacheConns cachedClientConnPool
var http2Transport = &http2.Transport{}

type http2ClientMuxStream struct {
	w         *io.PipeWriter
	r         io.ReadCloser
	readReady chan struct{}
	closed    bool
}

func (s *http2ClientMuxStream) setReader(rr io.ReadCloser) {
	s.r = rr
	helper.AsyncNotify(s.readReady)
}

func (s *http2ClientMuxStream) Read(b []byte) (n int, err error) {
	for s.r == nil && !s.closed {
		select {
		case <-s.readReady:
		case <-time.After(30 * time.Second):
			return 0, pmux.ErrTimeout
		}
	}
	if s.closed {
		return 0, io.EOF
	}
	n, err = s.r.Read(b)
	return
}

func (s *http2ClientMuxStream) Write(b []byte) (n int, err error) {
	if s.closed {
		return 0, io.EOF
	}
	n, err = s.w.Write(b)
	return
}

func (s *http2ClientMuxStream) Close() (err error) {
	s.closed = true
	s.w.Close()
	if nil != s.r {
		s.r.Close()
	}
	helper.AsyncNotify(s.readReady)
	return nil
}

type HTTP2MuxSession struct {
	streamCounter int64
	net.Conn
	h2Conn     *http2.ClientConn
	tr         *http2.Transport
	ServerHost string
	//Client     *http.Client
	//Client        *http2.Transport
	AcceptCh chan MuxStream
	closeCh  chan struct{}
	streams  sync.Map
}

func (s *HTTP2MuxSession) RemoteAddr() net.Addr {
	return nil
}
func (s *HTTP2MuxSession) LocalAddr() net.Addr {
	return nil
}

func (q *HTTP2MuxSession) CloseStream(stream MuxStream) error {
	q.streams.Delete(stream)
	atomic.AddInt64(&q.streamCounter, -1)
	return nil
}

func (q *HTTP2MuxSession) OfferStream(stream TimeoutReadWriteCloser) error {
	s := &ProxyMuxStream{TimeoutReadWriteCloser: stream, session: q}
	select {
	case q.AcceptCh <- s:
		return nil
	default:
		stream.Close()
		return fmt.Errorf("Can NOT accept new stream")
	}
}

func (q *HTTP2MuxSession) Ping() (time.Duration, error) {
	start := time.Now()
	if nil != q.h2Conn {
		q.h2Conn.Ping(context.Background())
		return time.Now().Sub(start), nil
	}
	return 0, nil
}

func (q *HTTP2MuxSession) OpenStream() (MuxStream, error) {
	if nil == q.Conn {
		return nil, pmux.ErrSessionShutdown
	}
	pr, pw := io.Pipe()
	req := &http.Request{
		Method:        http.MethodPost,
		URL:           &url.URL{Scheme: "https", Host: q.ServerHost},
		Header:        make(http.Header),
		Proto:         "HTTP/2.0",
		ProtoMajor:    2,
		ProtoMinor:    0,
		Body:          pr,
		Host:          q.ServerHost,
		ContentLength: -1,
	}
	stream := &http2ClientMuxStream{
		w:         pw,
		readReady: make(chan struct{}),
	}
	go func() {
		//opt := http2.RoundTripOpt{OnlyCachedConn: true}
		res, err := q.h2Conn.RoundTrip(req)
		//res, err := q.Client.Do(req)
		if nil != err {
			logger.Error("Failed to post http/2 with error:%v", err)
			stream.Close()
			q.Close()
		} else {
			stream.setReader(res.Body)
		}
	}()
	muxStream := &ProxyMuxStream{TimeoutReadWriteCloser: &helper.TimeoutReadWriteCloser{ReadWriteCloser: stream}, session: q}
	atomic.AddInt64(&q.streamCounter, 1)
	q.streams.Store(muxStream, true)
	return muxStream, nil
}

func (q *HTTP2MuxSession) AcceptStream() (MuxStream, error) {
	select {
	case conn := <-q.AcceptCh:
		q.streams.Store(conn, true)
		return conn, nil
	case <-q.closeCh:
		return nil, pmux.ErrSessionShutdown
	}
}

func (q *HTTP2MuxSession) NumStreams() int {
	return int(q.streamCounter)
}

func (q *HTTP2MuxSession) Close() error {
	helper.AsyncNotify(q.closeCh)
	if nil != q.Conn {
		q.Conn.Close()
		q.Conn = nil
	}
	q.streams.Range(func(key, value interface{}) bool {
		stream := key.(MuxStream)
		stream.Close()
		return true
	})
	http2Transport.CloseIdleConnections()
	//q.streams = sync.Map{}
	return nil
}

func NewHTTP2ServerMuxSession(conn net.Conn) *HTTP2MuxSession {
	s := &HTTP2MuxSession{}
	s.AcceptCh = make(chan MuxStream, 64)
	s.closeCh = make(chan struct{})
	s.Conn = conn
	return s
}

func NewHTTP2ClientMuxSession(conn net.Conn, host string) (MuxSession, error) {
	s := &HTTP2MuxSession{}
	s.Conn = conn
	s.closeCh = make(chan struct{})
	cc, err := http2Transport.NewClientConn(conn)
	if nil != err {
		return nil, err
	}
	s.h2Conn = cc
	//cacheConns.add(cc)
	//tr.ConnPool = &singleClientConnPool{conn: cc}
	//client := &http.Client{}
	//client.Transport = tr
	//s.Client = tr
	//s.Client = client
	s.ServerHost = host
	return s, nil
}

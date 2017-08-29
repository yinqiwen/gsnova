package mux

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/pmux"
	"golang.org/x/net/http2"
)

type singleClientConnPool struct {
	conn *http2.ClientConn
}

func (c *singleClientConnPool) GetClientConn(req *http.Request, addr string) (*http2.ClientConn, error) {
	//log.Printf("###return %v %v %p", req, addr, c.conn)
	if nil != c.conn {
		return c.conn, nil
	}
	return nil, fmt.Errorf("Existing connection closed")
}
func (c *singleClientConnPool) MarkDead(conn *http2.ClientConn) {
	c.conn = nil
}

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
	net.Conn
	ServerHost string
	//Client        *http.Client
	Client        *http2.Transport
	streamCounter int64
	AcceptCh      chan MuxStream
	closeCh       chan struct{}
}

func (q *HTTP2MuxSession) CloseStream(stream MuxStream) error {
	atomic.AddInt64(&q.streamCounter, -1)
	return nil
}

func (q *HTTP2MuxSession) OfferStream(stream MuxStream) error {
	select {
	case q.AcceptCh <- stream:
		return nil
	default:
		stream.Close()
		return fmt.Errorf("Can NOT accept new stream")
	}
}

func (q *HTTP2MuxSession) OpenStream() (MuxStream, error) {
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
		opt := http2.RoundTripOpt{OnlyCachedConn: false}
		res, err := q.Client.RoundTripOpt(req, opt)
		if nil != err {
			stream.Close()
		} else {
			stream.setReader(res.Body)
		}
	}()

	return &ProxyMuxStream{ReadWriteCloser: stream, session: q}, nil
}

func (q *HTTP2MuxSession) AcceptStream() (MuxStream, error) {
	select {
	case conn := <-q.AcceptCh:
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
	return q.Conn.Close()
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
	s.closeCh = make(chan struct{})
	tr := &http2.Transport{}
	cc, err := tr.NewClientConn(conn)
	if nil != err {
		return nil, err
	}
	tr.ConnPool = &singleClientConnPool{conn: cc}
	//client := &http.Client{}
	//client.Transport = tr
	s.Client = tr
	s.ServerHost = host
	return s, nil
}

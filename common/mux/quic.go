package mux

import (
	"context"
	"net"
	"sync/atomic"
	"time"

	quic "github.com/lucas-clemente/quic-go"
)

type QUICMuxSession struct {
	streamCounter int64
	quic.Session
}

func (q *QUICMuxSession) Ping() (time.Duration, error) {
	return 0, nil
}

func (q *QUICMuxSession) CloseStream(stream MuxStream) error {
	atomic.AddInt64(&q.streamCounter, -1)
	return nil
}

func (q *QUICMuxSession) OpenStream() (MuxStream, error) {
	s, err := q.OpenStreamSync(context.Background())
	if nil != err {
		return nil, err
	}
	atomic.AddInt64(&q.streamCounter, 1)
	return &ProxyMuxStream{TimeoutReadWriteCloser: s, session: q}, nil
}

func (q *QUICMuxSession) AcceptStream() (MuxStream, error) {
	s, err := q.Session.AcceptStream(context.Background())
	if nil != err {
		return nil, err
	}
	return &ProxyMuxStream{TimeoutReadWriteCloser: s}, nil
}

func (q *QUICMuxSession) NumStreams() int {
	return int(q.streamCounter)
}

func (q *QUICMuxSession) Close() error {
	q.streamCounter = 0
	return q.Session.CloseWithError(0, "")
}
func (s *QUICMuxSession) RemoteAddr() net.Addr {
	return nil
}
func (s *QUICMuxSession) LocalAddr() net.Addr {
	return nil
}

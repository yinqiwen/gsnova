package mux

import (
	"sync/atomic"

	quic "github.com/lucas-clemente/quic-go"
)

type QUICMuxSession struct {
	quic.Session
	streamCounter int64
}

func (q *QUICMuxSession) CloseStream(stream MuxStream) error {
	atomic.AddInt64(&q.streamCounter, -1)
	return nil
}

func (q *QUICMuxSession) OpenStream() (MuxStream, error) {
	s, err := q.OpenStreamSync()
	if nil != err {
		return nil, err
	}
	atomic.AddInt64(&q.streamCounter, 1)
	return &ProxyMuxStream{ReadWriteCloser: s, session: q}, nil
}

func (q *QUICMuxSession) AcceptStream() (MuxStream, error) {
	s, err := q.Session.AcceptStream()
	if nil != err {
		return nil, err
	}
	return &ProxyMuxStream{ReadWriteCloser: s}, nil
}

func (q *QUICMuxSession) NumStreams() int {
	return int(q.streamCounter)
}

func (q *QUICMuxSession) Close() error {
	return q.Session.Close(nil)
}

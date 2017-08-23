package mux

import (
	quic "github.com/lucas-clemente/quic-go"
)

type QUICMuxSession struct {
	quic.Session
}

func (q *QUICMuxSession) OpenStream() (MuxStream, error) {
	s, err := q.OpenStreamSync()
	if nil != err {
		return nil, err
	}
	return &ProxyMuxStream{s}, nil
}

func (q *QUICMuxSession) AcceptStream() (MuxStream, error) {
	s, err := q.Session.AcceptStream()
	if nil != err {
		return nil, err
	}
	return &ProxyMuxStream{s}, nil
}

func (q *QUICMuxSession) NumStreams() int {
	return -1
}

func (q *QUICMuxSession) Close() error {
	return q.Session.Close(nil)
}

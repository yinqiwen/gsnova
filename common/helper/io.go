package helper

import (
	"bytes"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

type PeekReader interface {
	Peek(n int) ([]byte, error)
}

type BufferChunkReader struct {
	io.Reader
	Err error
}

func (r *BufferChunkReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.Err = err
	if nil != err {
		return n, err
	}
	return n, io.EOF
}

type DebugReader struct {
	io.Reader
	Buf bytes.Buffer
}

func (r *DebugReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.Buf.Write(p[0:n])
	}
	return n, err
}

func IsTimeoutError(err error) bool {
	if err, ok := err.(net.Error); ok && err.Timeout() {
		return true
	}
	return false
}

type TimeoutReadWriteCloser struct {
	io.ReadWriteCloser
	readDeadline  time.Time
	writeDeadline time.Time
}

func (s *TimeoutReadWriteCloser) SetReadDeadline(t time.Time) error {
	s.readDeadline = t
	return nil
}
func (s *TimeoutReadWriteCloser) SetWriteDeadline(t time.Time) error {
	s.writeDeadline = t
	return nil
}
func (s *TimeoutReadWriteCloser) Read(p []byte) (n int, err error) {
	var timeout <-chan time.Time
	if !s.readDeadline.IsZero() {
		delay := s.readDeadline.Sub(time.Now())
		timeout = time.After(delay)
	} else {
		return s.ReadWriteCloser.Read(p)
	}
	done := make(chan bool, 1)
	go func() {
		n, err = s.ReadWriteCloser.Read(p)
		done <- true
	}()
	select {
	case <-done:
		return
	case <-timeout:
		return 0, ErrReadTimeout
	}
}

func (s *TimeoutReadWriteCloser) Write(p []byte) (n int, err error) {
	var timeout <-chan time.Time
	if !s.writeDeadline.IsZero() {
		delay := s.writeDeadline.Sub(time.Now())
		timeout = time.After(delay)
	} else {
		return s.ReadWriteCloser.Write(p)
	}
	done := make(chan bool, 1)
	go func() {
		n, err = s.ReadWriteCloser.Write(p)
		done <- true
	}()
	select {
	case <-done:
		return
	case <-timeout:
		return 0, ErrWriteTimeout
	}
}

type HttpPostWriter struct {
	URL string
}

func (s *HttpPostWriter) Write(p []byte) (n int, err error) {
	res, err := http.Post(s.URL, "text/plain", bytes.NewBuffer(p))
	if nil != err {
		log.Printf("Post error:%v", err)
		return 0, err
	}
	if nil != res.Body {
		res.Body.Close()
	}
	return len(p), nil
}

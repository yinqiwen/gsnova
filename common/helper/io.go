package helper

import (
	"bytes"
	"io"
	"net"
)

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

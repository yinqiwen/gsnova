package helper

import (
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

func IsTimeoutError(err error) bool {
	if err, ok := err.(net.Error); ok && err.Timeout() {
		return true
	}
	return false
}

package helper

import "io"

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

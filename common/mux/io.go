package mux

import (
	"io"

	"github.com/golang/snappy"
)

const (
	SnappyCompressor = "snappy"
	NoneCompressor   = "none"
)

func GetCompressStreamReaderWriter(stream MuxStream, method string) (io.Reader, io.Writer) {
	switch method {
	case SnappyCompressor:
		return snappy.NewReader(stream), snappy.NewWriter(stream)
	case NoneCompressor:
		fallthrough
	default:
		return stream, stream
	}
}

func IsValidCompressor(method string) bool {
	switch method {
	case SnappyCompressor:
	case NoneCompressor:
	default:
		return false
	}
	return true
}

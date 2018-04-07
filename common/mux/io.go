package mux

import (
	"io"
	"net"
	"time"

	"github.com/golang/snappy"
)

const (
	SnappyCompressor = "snappy"
	NoneCompressor   = "none"
)

func GetCompressStreamReaderWriter(stream io.ReadWriteCloser, method string) (io.Reader, io.Writer) {
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

type MuxStreamConn struct {
	MuxStream
}

func (s *MuxStreamConn) LocalAddr() net.Addr {
	return nil
}

func (s *MuxStreamConn) RemoteAddr() net.Addr {
	return nil

}
func (s *MuxStreamConn) SetDeadline(t time.Time) error {
	s.MuxStream.SetReadDeadline(t)
	s.MuxStream.SetWriteDeadline(t)
	return nil
}

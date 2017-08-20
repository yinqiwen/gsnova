package proxy

import (
	"io"

	"github.com/vmihailenco/msgpack"
)

type ConnectRequest struct {
	//ProxySID uint32
	Network string
	Addr    string
}

type AuthRequest struct {
	Rand          []byte
	User          string
	CipherCounter uint64
	CipherMethod  string
}
type AuthResponse struct {
	Code int
}

func ReadConnectRequest(stream io.Reader) (*ConnectRequest, error) {
	dec := msgpack.NewDecoder(stream)
	var q ConnectRequest
	err := dec.Decode(&q)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

func ReadAuthRequest(stream io.Reader) (*AuthRequest, error) {
	dec := msgpack.NewDecoder(stream)
	var q AuthRequest
	err := dec.Decode(&q)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

func WriteMessage(stream io.Writer, req interface{}) error {
	enc := msgpack.NewEncoder(stream)
	return enc.Encode(req)
}

type MuxStream interface {
	io.ReadWriteCloser
	Connect(network string, addr string) error
	Auth(user string) error
}

type MuxSession interface {
	OpenStream() (MuxStream, error)
	NumStreams() int
	Close() error
}

func init() {
	msgpack.RegisterExt(1, (*AuthRequest)(nil))
	msgpack.RegisterExt(2, (*AuthResponse)(nil))
	msgpack.RegisterExt(3, (*ConnectRequest)(nil))
	//msgpack.RegisterExt(1, (*B)(nil))
}

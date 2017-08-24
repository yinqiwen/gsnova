package mux

import (
	"bytes"
	"encoding/binary"
	"io"
	"io/ioutil"
	"math/rand"
	"time"

	quic "github.com/lucas-clemente/quic-go"
	"github.com/vmihailenco/msgpack"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/pmux"
)

type ConnectRequest struct {
	//ProxySID uint32
	Network string
	Addr    string
}

type AuthRequest struct {
	Rand          string
	User          string
	CipherCounter uint64
	CipherMethod  string
}
type AuthResponse struct {
	Code int
}

func ReadConnectRequest(stream io.Reader) (*ConnectRequest, error) {
	var q ConnectRequest
	err := ReadMessage(stream, &q)
	return &q, err
}

func ReadAuthRequest(stream io.Reader) (*AuthRequest, error) {
	var q AuthRequest
	err := ReadMessage(stream, &q)
	return &q, err
}

func WriteMessage(stream io.Writer, req interface{}) error {
	buf := &bytes.Buffer{}
	buf.Write([]byte{0, 0, 0, 0})
	enc := msgpack.NewEncoder(buf)
	err := enc.Encode(req)
	if nil != err {
		return err
	}
	binary.BigEndian.PutUint32(buf.Bytes(), uint32(buf.Len()-4))
	_, err = stream.Write(buf.Bytes())
	return err
}

func ReadMessage(stream io.Reader, res interface{}) error {
	lenbuf := make([]byte, 4)
	n, err := io.ReadAtLeast(stream, lenbuf, len(lenbuf))
	length := uint32(0)
	if n == len(lenbuf) {
		length = binary.BigEndian.Uint32(lenbuf)
		if length > 1000000 {
			return ErrToolargeMessage
		}
	} else {
		return err
	}

	buf := make([]byte, length)
	n, err = io.ReadAtLeast(stream, buf, len(buf))
	if n == len(buf) {
		dec := msgpack.NewDecoder(bytes.NewBuffer(buf))
		return dec.Decode(res)
	}
	return err
}

type MuxStream interface {
	io.ReadWriteCloser
	Connect(network string, addr string) error
	Auth(user string, cipherMethod string, cipherCounter uint64) error
	StreamID() uint32
}

type MuxSession interface {
	OpenStream() (MuxStream, error)
	CloseStream(stream MuxStream) error
	AcceptStream() (MuxStream, error)
	NumStreams() int
	Close() error
}

type ProxyMuxStream struct {
	io.ReadWriteCloser
	session MuxSession
}

func (s *ProxyMuxStream) StreamID() uint32 {
	if ps, ok := s.ReadWriteCloser.(*pmux.Stream); ok {
		return ps.ID()
	} else if qs, ok := s.ReadWriteCloser.(quic.Stream); ok {
		return uint32(qs.StreamID())
	}
	return 0
}

func (s *ProxyMuxStream) Close() error {
	if nil != s.session {
		s.session.CloseStream(s)
	}
	return s.ReadWriteCloser.Close()
}

func (s *ProxyMuxStream) Connect(network string, addr string) error {
	req := &ConnectRequest{Network: network, Addr: addr}
	return WriteMessage(s, req)
}
func (s *ProxyMuxStream) Auth(user string, cipherMethod string, cipherCounter uint64) error {
	req := &AuthRequest{}
	req.CipherCounter = cipherCounter
	req.CipherMethod = cipherMethod
	req.User = user
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	req.Rand = helper.RandAsciiString(int(r.Int31n(128)))
	err := WriteMessage(s, req)
	if nil != err {
		return err
	}
	res := &AuthResponse{}
	err = ReadMessage(s, res)
	if nil != err {
		return err
	}
	if nil == err {
		//wait remote close
		ioutil.ReadAll(s)
	}
	//s.Read(make([]byte, 1))
	if res.Code != AuthOK {
		return ErrAuthFailed
	}
	return nil
}

type ProxyMuxSession struct {
	*pmux.Session
}

func (s *ProxyMuxSession) CloseStream(stream MuxStream) error {
	return nil
}

func (s *ProxyMuxSession) OpenStream() (MuxStream, error) {
	ss, err := s.Session.OpenStream()
	if nil != err {
		return nil, err
	}
	return &ProxyMuxStream{ReadWriteCloser: ss}, nil
}

func (s *ProxyMuxSession) AcceptStream() (MuxStream, error) {
	ss, err := s.Session.AcceptStream()
	if nil != err {
		return nil, err
	}
	return &ProxyMuxStream{ReadWriteCloser: ss}, nil
}

func init() {
	//msgpack.RegisterExt(1, (*AuthRequest)(nil))
	// msgpack.RegisterExt(2, (*AuthResponse)(nil))
	// msgpack.RegisterExt(3, (*ConnectRequest)(nil))
	//msgpack.RegisterExt(1, (*B)(nil))
}

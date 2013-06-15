package event

import (
	"bytes"
)

type SocketReadEvent struct {
	Timeout uint32
	MaxRead uint32
	EventHeader
}

func (req *SocketReadEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt32Value(buffer, req.Timeout)
	EncodeUInt32Value(buffer, req.MaxRead)
}
func (req *SocketReadEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.Timeout, err = DecodeUInt32Value(buffer)
	req.MaxRead, err = DecodeUInt32Value(buffer)
	return
}

func (req *SocketReadEvent) GetType() uint32 {
	return EVENT_SOCKET_READ_TYPE
}
func (req *SocketReadEvent) GetVersion() uint32 {
	return 1
}

type SocketConnectWithDataEvent struct {
	Content []byte
	Addr    string
	Net     string
	Timeout uint32
	EventHeader
}

func (req *SocketConnectWithDataEvent) Encode(buffer *bytes.Buffer) {
	EncodeBytesValue(buffer, req.Content)
	EncodeStringValue(buffer, req.Addr)
	EncodeStringValue(buffer, req.Net)
	EncodeUInt32Value(buffer, req.Timeout)
}
func (req *SocketConnectWithDataEvent) Decode(buffer *bytes.Buffer) (err error) {
	if req.Content, err = DecodeBytesValue(buffer); nil == err {
		if req.Addr, err = DecodeStringValue(buffer); nil == err {
			if req.Net, err = DecodeStringValue(buffer); nil == err {
				req.Timeout, err = DecodeUInt32Value(buffer)
			}
		}
	}
	return
}

func (req *SocketConnectWithDataEvent) GetType() uint32 {
	return EVENT_SOCKET_CONNECT_WITH_DATA_TYPE
}
func (req *SocketConnectWithDataEvent) GetVersion() uint32 {
	return 1
}

type RSocketAcceptedEvent struct {
	Server string
	EventHeader
}

func (req *RSocketAcceptedEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, req.Server)
}
func (req *RSocketAcceptedEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.Server, err = DecodeStringValue(buffer)
	if err != nil {
		return
	}
	return nil
}

func (req *RSocketAcceptedEvent) GetType() uint32 {
	return EVENT_RSOCKET_ACCEPTED_TYPE
}
func (req *RSocketAcceptedEvent) GetVersion() uint32 {
	return 1
}

type TCPChunkEvent struct {
	Sequence uint32
	Content  []byte
	EventHeader
}

func (req *TCPChunkEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt32Value(buffer, req.Sequence)
	EncodeBytesValue(buffer, req.Content)
}
func (req *TCPChunkEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.Sequence, err = DecodeUInt32Value(buffer)
	if err != nil {
		return
	}
	req.Content, err = DecodeBytesValue(buffer)
	if err != nil {
		return
	}
	return nil
}

func (req *TCPChunkEvent) GetType() uint32 {
	return EVENT_TCP_CHUNK_TYPE
}
func (req *TCPChunkEvent) GetVersion() uint32 {
	return 1
}

type SocketConnectionEvent struct {
	Status uint32
	Addr   string
	EventHeader
}

func (req *SocketConnectionEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt32Value(buffer, req.Status)
	EncodeStringValue(buffer, req.Addr)
}
func (req *SocketConnectionEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.Status, err = DecodeUInt32Value(buffer)
	if err != nil {
		return
	}
	req.Addr, err = DecodeStringValue(buffer)
	if err != nil {
		return
	}
	return nil
}

func (req *SocketConnectionEvent) GetType() uint32 {
	return EVENT_TCP_CONNECTION_TYPE
}
func (req *SocketConnectionEvent) GetVersion() uint32 {
	return 1
}

type UserLoginEvent struct {
	User string
	EventHeader
}

func (req *UserLoginEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, req.User)
}
func (req *UserLoginEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.User, err = DecodeStringValue(buffer)
	if err != nil {
		return
	}
	return nil
}

func (req *UserLoginEvent) GetType() uint32 {
	return EVENT_USER_LOGIN_TYPE
}
func (req *UserLoginEvent) GetVersion() uint32 {
	return 1
}

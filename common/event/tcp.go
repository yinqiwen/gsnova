package event

import (
	"bytes"
)

type TCPChunkEvent struct {
	EventHeader
	Content []byte
}

func (ev *TCPChunkEvent) Encode(buffer *bytes.Buffer) {
	EncodeBytesValue(buffer, ev.Content)
}
func (ev *TCPChunkEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.Content, err = DecodeBytesValue(buffer)
	return
}

type TCPOpenEvent struct {
	EventHeader
	Addr string
}

func (ev *TCPOpenEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, ev.Addr)
}
func (ev *TCPOpenEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.Addr, err = DecodeStringValue(buffer)
	return
}

type TCPCloseEvent struct {
	EventHeader
}

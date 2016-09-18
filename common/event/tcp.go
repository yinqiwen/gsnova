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

type ConnCloseEvent struct {
	EventHeader
}

func (ev *ConnCloseEvent) Encode(buffer *bytes.Buffer) {
}
func (ev *ConnCloseEvent) Decode(buffer *bytes.Buffer) (err error) {
	return nil
}

type ConnTestEvent struct {
	EventHeader
}

func (ev *ConnTestEvent) Encode(buffer *bytes.Buffer) {
}
func (ev *ConnTestEvent) Decode(buffer *bytes.Buffer) (err error) {
	return nil
}

type PortUnicastEvent struct {
	EventHeader
	Port uint32
}

func (ev *PortUnicastEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt32Value(buffer, ev.Port)
}
func (ev *PortUnicastEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.Port, err = DecodeUInt32Value(buffer)
	return
}

package event

import (
	"bytes"
)

type UDPEvent struct {
	EventHeader
	Addr    string
	Content []byte
}

func (ev *UDPEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, ev.Addr)
	EncodeBytesValue(buffer, ev.Content)
}
func (ev *UDPEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.Addr, err = DecodeStringValue(buffer)
	if nil != err {
		return
	}
	ev.Content, err = DecodeBytesValue(buffer)
	return
}

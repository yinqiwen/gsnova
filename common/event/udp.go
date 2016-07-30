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
	EncodeBytesValue(buffer, ev.Content)
}
func (ev *UDPEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.Content, err = DecodeBytesValue(buffer)
	return
}

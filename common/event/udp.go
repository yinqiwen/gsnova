package event

import (
	"bytes"
)

type UDPEvent struct {
	EventHeader
	SrcIp   int64
	SrcPort uint16
	DstIp   int64
	DstPort uint16
	Content []byte
}

func (ev *UDPEvent) Encode(buffer *bytes.Buffer) {
	EncodeBytesValue(buffer, ev.Content)
}
func (ev *UDPEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.Content, err = DecodeBytesValue(buffer)
	return
}

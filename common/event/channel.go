package event

import "bytes"

type ChannelCloseReqEvent struct {
	EventHeader
}

func (ev *ChannelCloseReqEvent) Encode(buffer *bytes.Buffer) {
}
func (ev *ChannelCloseReqEvent) Decode(buffer *bytes.Buffer) (err error) {
	return nil
}

type ChannelCloseACKEvent struct {
	EventHeader
}

func (ev *ChannelCloseACKEvent) Encode(buffer *bytes.Buffer) {
}
func (ev *ChannelCloseACKEvent) Decode(buffer *bytes.Buffer) (err error) {
	return nil
}

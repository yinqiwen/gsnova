package event

import (
	"bytes"
)

type HeartBeatEvent struct {
	EventHeader
}

func (ev *HeartBeatEvent) Encode(buffer *bytes.Buffer) {
}
func (ev *HeartBeatEvent) Decode(buffer *bytes.Buffer) (err error) {
	return nil
}

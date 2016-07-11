package event

import (
	"bytes"
)

type ErrorEvent struct {
	EventHeader
	Code   int64
	Reason string
}

func (ev *ErrorEvent) Encode(buffer *bytes.Buffer) {
	EncodeInt64Value(buffer, ev.Code)
}
func (ev *ErrorEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.Code, err = DecodeInt64Value(buffer)
	if nil == err {
		ev.Reason, err = DecodeStringValue(buffer)
	}
	return
}

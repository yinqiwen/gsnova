package event

import (
	"bytes"
)

const (
	ErrTooLargeResponse   = 1001
	ErrInvalidHttpRequest = 1002
	ErrRemoteProxyTimeout = 1003
	ErrAuthFailed         = 1004

	SuccessAuthed = 10000
)

type NotifyEvent struct {
	EventHeader
	Code   int64
	Reason string
}

func (ev *NotifyEvent) Encode(buffer *bytes.Buffer) {
	EncodeInt64Value(buffer, ev.Code)
	EncodeStringValue(buffer, ev.Reason)
}
func (ev *NotifyEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.Code, err = DecodeInt64Value(buffer)
	if nil == err {
		ev.Reason, err = DecodeStringValue(buffer)
	}
	return
}

package event

import (
	"bytes"
)

type AuthEvent struct {
	EventHeader
	User  string
	Mac   string
	Index int64
}

func (ev *AuthEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, ev.User)
	EncodeStringValue(buffer, ev.Mac)
	EncodeInt64Value(buffer, ev.Index)
}
func (ev *AuthEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.User, err = DecodeStringValue(buffer)
	if nil != err {
		return err
	}
	ev.Mac, err = DecodeStringValue(buffer)
	if nil != err {
		return err
	}
	ev.Index, err = DecodeInt64Value(buffer)
	return err
}

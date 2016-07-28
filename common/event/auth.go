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
	if nil != err {
		return err
	}
	return err
}

//Initial vector setting
type IVSettingEvent struct {
	IV []byte
}

func (ev *IVSettingEvent) Encode(buffer *bytes.Buffer) {
	EncodeBytesValue(buffer, ev.IV)
}
func (ev *IVSettingEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.IV, err = DecodeBytesValue(buffer)
	return err
}

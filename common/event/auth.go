package event

import (
	"bytes"
	"math/rand"
	"time"
)

var authRunId int64

type AuthEvent struct {
	EventHeader
	User  string
	Mac   string
	RunId int64
	Index int64
	IV    uint64
}

func (ev *AuthEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, ev.User)
	EncodeStringValue(buffer, ev.Mac)
	ev.RunId = authRunId
	EncodeInt64Value(buffer, ev.RunId)
	EncodeInt64Value(buffer, ev.Index)
	EncodeUInt64Value(buffer, ev.IV)
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
	ev.RunId, err = DecodeInt64Value(buffer)
	if nil != err {
		return err
	}
	ev.Index, err = DecodeInt64Value(buffer)
	if nil != err {
		return err
	}
	ev.IV, err = DecodeUInt64Value(buffer)
	return err
}

func init() {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	authRunId = r.Int63()
}

package event

import (
	"bytes"
	"math/rand"
	"time"
)

type HeartBeatEvent struct {
	EventHeader
	Rand []byte
}

func (ev *HeartBeatEvent) Encode(buffer *bytes.Buffer) {
	EncodeBytesValue(buffer, ev.Rand)
}
func (ev *HeartBeatEvent) Decode(buffer *bytes.Buffer) (err error) {
	return nil
}

func NewHeartBeatEvent() *HeartBeatEvent {
	hb := &HeartBeatEvent{}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	hb.SetId(uint32(r.Int31()))
	//hb.Rand = []byte(helper.RandAsciiString(int(r.Int31n(16))))
	return hb
}

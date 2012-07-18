package event

import (
	"bytes"
	//"fmt"
)

type RequestAllAppIDEvent struct {
	EventHeader
}

func (ev *RequestAllAppIDEvent) Encode(buffer *bytes.Buffer) {
}
func (ev *RequestAllAppIDEvent) Decode(buffer *bytes.Buffer) error {
	return nil
}

func (ev *RequestAllAppIDEvent) GetType() uint32 {
	return REQUEST_ALL_SHARED_APPID_EVENT_TYPE
}
func (ev *RequestAllAppIDEvent) GetVersion() uint32 {
	return 1
}

type RequestAppIDEvent struct {
	EventHeader
}

func (ev *RequestAppIDEvent) Encode(buffer *bytes.Buffer) {
}
func (ev *RequestAppIDEvent) Decode(buffer *bytes.Buffer) error {
	return nil
}

func (ev *RequestAppIDEvent) GetType() uint32 {
	return REQUEST_SHARED_APPID_EVENT_TYPE
}
func (ev *RequestAppIDEvent) GetVersion() uint32 {
	return 1
}

type RequestAppIDResponseEvent struct {
	AppIDs []string
	EventHeader
}

func (ev *RequestAppIDResponseEvent) Encode(buffer *bytes.Buffer) {
	if nil == ev.AppIDs {
		EncodeInt64Value(buffer, 0)
		return
	}
	EncodeUInt64Value(buffer, uint64(len(ev.AppIDs)))
	for _, appid := range ev.AppIDs {
		EncodeStringValue(buffer, appid)
	}
}
func (ev *RequestAppIDResponseEvent) Decode(buffer *bytes.Buffer) error {
	tmp, err := DecodeUInt64Value(buffer)
	if err != nil {
		return err
	}
	//fmt.Printf("len=%d\n", tmp)
	ev.AppIDs = make([]string, int(tmp))
	//var ok bool
	for i := 0; i < int(tmp); i++ {
		ev.AppIDs[i], err = DecodeStringValue(buffer)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ev *RequestAppIDResponseEvent) GetType() uint32 {
	return REQUEST_SHARED_APPID_RESULT_EVENT_TYPE
}
func (ev *RequestAppIDResponseEvent) GetVersion() uint32 {
	return 1
}

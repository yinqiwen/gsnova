package event

import (
	"bytes"
	"encoding/binary"
)

type EventHeaderTags struct {
	magic uint16
	Token string
}

func (tags *EventHeaderTags) Encode(buffer *bytes.Buffer) {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, MAGIC_NUMBER)
	buffer.Write(b)
	EncodeStringValue(buffer, tags.Token)
}
func (tags *EventHeaderTags) Decode(buffer *bytes.Buffer) bool {
	b := make([]byte, 2)
	realLen, err := buffer.Read(b)
	if err != nil || realLen != 2 {
		return false
	}
	tags.magic = binary.BigEndian.Uint16(b)
	if tags.magic != MAGIC_NUMBER {
		return false
	}
	token, ok := DecodeStringValue(buffer)
	tags.Token = token
	return ok == nil
}

type AuthRequestEvent struct {
	Appid  string
	User   string
	Passwd string
	EventHeader
}

func (req *AuthRequestEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, req.Appid)
	EncodeStringValue(buffer, req.User)
	EncodeStringValue(buffer, req.Passwd)
}
func (req *AuthRequestEvent) Decode(buffer *bytes.Buffer) error {
	var err error
	req.Appid, err = DecodeStringValue(buffer)
	if nil != err {
		return err
	}
	req.User, err = DecodeStringValue(buffer)
	if nil != err {
		return err
	}
	req.Passwd, err = DecodeStringValue(buffer)
	if nil != err {
		return err
	}
	return nil
}

func (req *AuthRequestEvent) GetType() uint32 {
	return AUTH_REQUEST_EVENT_TYPE
}
func (req *AuthRequestEvent) GetVersion() uint32 {
	return 1
}

type AuthResponseEvent struct {
	Appid      string
	Token      string
	Error      string
	Capability uint64
	EventHeader
}

func (req *AuthResponseEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, req.Appid)
	EncodeStringValue(buffer, req.Token)
	EncodeStringValue(buffer, req.Error)
	EncodeUInt64Value(buffer, req.Capability)
}
func (req *AuthResponseEvent) Decode(buffer *bytes.Buffer) error {
	var err error
	req.Appid, err = DecodeStringValue(buffer)
	if nil != err {
		return err
	}
	req.Token, err = DecodeStringValue(buffer)
	if nil != err {
		return err
	}
	req.Error, err = DecodeStringValue(buffer)
	if nil != err {
		return err
	}
	req.Capability, _ = DecodeUInt64Value(buffer)
	return nil
}
func (req *AuthResponseEvent) GetType() uint32 {
	return AUTH_RESPONSE_EVENT_TYPE
}
func (req *AuthResponseEvent) GetVersion() uint32 {
	return 1
}

type AdminResponseEvent struct {
	Response   string
	ErrorCause string
	errno      int32
	EventHeader
}

func (res *AdminResponseEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, res.Response)
	EncodeStringValue(buffer, res.ErrorCause)
	EncodeUInt64Value(buffer, uint64(res.errno))
}
func (res *AdminResponseEvent) Decode(buffer *bytes.Buffer) error {
	var err error
	res.Response, err = DecodeStringValue(buffer)
	if nil != err {
		return err
	}
	res.ErrorCause, err = DecodeStringValue(buffer)
	if nil != err {
		return err
	}
	tmp, err := DecodeUInt64Value(buffer)
	if err != nil {
		return err
	}
	res.errno = int32(tmp)
	return nil
}
func (res *AdminResponseEvent) GetType() uint32 {
	return ADMIN_RESPONSE_EVENT_TYPE
}
func (res *AdminResponseEvent) GetVersion() uint32 {
	return 1
}

//const (
//	APPID_SHARE   uint32 = 0
//	APPID_UNSHARE uint32 = 1
//)
//
//type ShareAppIDEvent struct {
//	Operation uint32
//	AppId     string
//	Email     string
//	EventHeader
//}
//
//func (ev *ShareAppIDEvent) Encode(buffer *bytes.Buffer) {
//	codec.WriteUvarint(buffer, uint64(ev.Operation))
//	codec.WriteVarString(buffer, ev.AppId)
//	codec.WriteVarString(buffer, ev.Email)
//	return true
//}
//func (ev *ShareAppIDEvent) Decode(buffer *bytes.Buffer) bool {
//	tmp, err := codec.ReadUvarint(buffer)
//	if err != nil {
//		return false
//	}
//	ev.Operation = uint32(tmp)
//	var ok bool
//	ev.AppId, ok = codec.ReadVarString(buffer)
//	if !ok {
//		return false
//	}
//	ev.Email, ok = codec.ReadVarString(buffer)
//	if !ok {
//		return false
//	}
//	return true
//}
//
//func (ev *ShareAppIDEvent) GetType() uint32 {
//	return SHARE_APPID_EVENT_TYPE
//}
//func (ev *ShareAppIDEvent) GetVersion() uint32 {
//	return 1
//}

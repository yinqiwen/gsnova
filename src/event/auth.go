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
	Appid string
	Token string
	Error string
	EventHeader
}

func (req *AuthResponseEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, req.Appid)
	EncodeStringValue(buffer, req.Token)
	EncodeStringValue(buffer, req.Error)
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
	return nil
}
func (req *AuthResponseEvent) GetType() uint32 {
	return AUTH_RESPONSE_EVENT_TYPE
}
func (req *AuthResponseEvent) GetVersion() uint32 {
	return 1
}

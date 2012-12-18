package event

import (
	"bytes"
	"crypto/rc4"
	"errors"
	"strconv"
	"util"
)

var rc4Key string

func SetRC4Key(key string) {
	rc4Key = key
}

type EncryptEvent struct {
	EncryptType uint32
	Ev          Event
	EventHeader
}

func (ev *EncryptEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt64Value(buffer, uint64(ev.EncryptType))
	buf := new(bytes.Buffer)
	EncodeEvent(buf, ev.Ev)
	switch ev.EncryptType {
	case ENCRYPTER_NONE:
		buffer.Write(buf.Bytes())
	case ENCRYPTER_SE1:
		newbuf := util.SimpleEncrypt(buf)
		buffer.Write(newbuf.Bytes())
		newbuf.Reset()
	case ENCRYPTER_RC4:
		dst := make([]byte, buf.Len())
		cipher, _ := rc4.NewCipher([]byte(rc4Key))
		cipher.XORKeyStream(dst, buf.Bytes())
		buffer.Write(dst)
	}
	buf.Reset()
}
func (ev *EncryptEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.EncryptType, err = DecodeUInt32Value(buffer)
	if err != nil {
		return err
	}
	switch ev.EncryptType {
	case ENCRYPTER_NONE:
		err, ev.Ev = DecodeEvent(buffer)
		return err
	case ENCRYPTER_SE1:
		newbuf := util.SimpleDecrypt(buffer)
		err, ev.Ev = DecodeEvent(newbuf)
		newbuf.Reset()
	case ENCRYPTER_RC4:
		dst := make([]byte, buffer.Len())
		cipher, _ := rc4.NewCipher([]byte(rc4Key))
		cipher.XORKeyStream(dst, buffer.Bytes())
		err, ev.Ev = DecodeEvent(bytes.NewBuffer(dst))
	default:
		return errors.New("Not supported encrypt type:" + strconv.Itoa(int(ev.EncryptType)))
	}
	return err
}

func (ev *EncryptEvent) GetType() uint32 {
	return ENCRYPT_EVENT_TYPE
}
func (ev *EncryptEvent) GetVersion() uint32 {
	return 1
}

type EncryptEventV2 struct {
	EncryptType uint32
	Ev          Event
	EventHeader
}

func (ev *EncryptEventV2) Encode(buffer *bytes.Buffer) {
	EncodeUInt64Value(buffer, uint64(ev.EncryptType))
	buf := new(bytes.Buffer)
	EncodeEvent(buf, ev.Ev)
	switch ev.EncryptType {
	case ENCRYPTER_NONE:
		EncodeUInt64Value(buffer, uint64(buf.Len()))
		buffer.Write(buf.Bytes())
	case ENCRYPTER_SE1:
		newbuf := util.SimpleEncrypt(buf)
		EncodeUInt64Value(buffer, uint64(newbuf.Len()))
		buffer.Write(newbuf.Bytes())
		newbuf.Reset()
	case ENCRYPTER_RC4:
		EncodeUInt64Value(buffer, uint64(buf.Len()))
		dst := make([]byte, buf.Len())
		cipher, _ := rc4.NewCipher([]byte(rc4Key))
		cipher.XORKeyStream(dst, buf.Bytes())
		buffer.Write(dst)
	}
	buf.Reset()
}
func (ev *EncryptEventV2) Decode(buffer *bytes.Buffer) (err error) {
	ev.EncryptType, err = DecodeUInt32Value(buffer)
	if err != nil {
		return err
	}
	length, err := DecodeUInt32Value(buffer)
	if err != nil {
		return
	}
	switch ev.EncryptType {
	case ENCRYPTER_NONE:
		err, ev.Ev = DecodeEvent(buffer)
		return err
	case ENCRYPTER_SE1:
		newbuf := util.SimpleDecrypt(bytes.NewBuffer(buffer.Next(int(length))))
		err, ev.Ev = DecodeEvent(newbuf)
		newbuf.Reset()
	case ENCRYPTER_RC4:
		src := buffer.Next(int(length))
		dst := make([]byte, int(length))
		cipher, _ := rc4.NewCipher([]byte(rc4Key))
		cipher.XORKeyStream(dst, src)
		err, ev.Ev = DecodeEvent(bytes.NewBuffer(dst))
	default:
		return errors.New("Not supported encrypt type:" + strconv.Itoa(int(ev.EncryptType)))
	}
	return err
}

func (ev *EncryptEventV2) GetType() uint32 {
	return ENCRYPT_EVENT_TYPE
}
func (ev *EncryptEventV2) GetVersion() uint32 {
	return 2
}

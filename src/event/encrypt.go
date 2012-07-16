package event

import (
	"errors"
	"strconv"
	"util"
	"bytes"
	//"fmt"
)

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
	}
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
		//fmt.Printf("Decrypt decode %d bytes\n", newbuf.Len())
		err, ev.Ev = DecodeEvent(newbuf)
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

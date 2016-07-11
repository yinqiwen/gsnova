package event

// import (
// 	"bytes"
// 	"crypto/rc4"
// 	"errors"
// 	"strconv"
// 	"util"
// )

// var rc4Key string

// func SetRC4Key(key string) {
// 	rc4Key = key
// }

// type EncryptEvent struct {
// 	EncryptType uint32
// 	Ev          Event
// 	EventHeader
// }

// func (ev *EncryptEvent) Encode(buffer *bytes.Buffer) {
// 	EncodeUInt64Value(buffer, uint64(ev.EncryptType))
// 	buf := new(bytes.Buffer)
// 	EncodeEvent(buf, ev.Ev)
// 	switch ev.EncryptType {
// 	case NoneEncypter:
// 		EncodeUInt64Value(buffer, uint64(buf.Len()))
// 		buffer.Write(buf.Bytes())
// 	case RC4Encypter:
// 		EncodeUInt64Value(buffer, uint64(buf.Len()))
// 		dst := make([]byte, buf.Len())
// 		cipher, _ := rc4.NewCipher([]byte(rc4Key))
// 		cipher.XORKeyStream(dst, buf.Bytes())
// 		buffer.Write(dst)
// 	}
// 	buf.Reset()
// }
// func (ev *EncryptEvent) Decode(buffer *bytes.Buffer) (err error) {
// 	ev.EncryptType, err = DecodeUInt32Value(buffer)
// 	if err != nil {
// 		return err
// 	}
// 	length, err := DecodeUInt32Value(buffer)
// 	if err != nil {
// 		return
// 	}
// 	switch ev.EncryptType {
// 	case NoneEncypter:
// 		err, ev.Ev = DecodeEvent(buffer)
// 		return err
// 	case RC4Encypter:
// 		src := buffer.Next(int(length))
// 		dst := make([]byte, int(length))
// 		cipher, _ := rc4.NewCipher([]byte(rc4Key))
// 		cipher.XORKeyStream(dst, src)
// 		err, ev.Ev = DecodeEvent(bytes.NewBuffer(dst))
// 	default:
// 		return errors.New("Not supported encrypt type:" + strconv.Itoa(int(ev.EncryptType)))
// 	}
// 	return err
// }

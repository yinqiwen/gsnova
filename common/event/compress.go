package event

// import (
// 	"bytes"
// 	"fmt"
// 	"github.com/golang/snappy"
// 	"strconv"
// )

// type CompressEvent struct {
// 	CompressType uint32
// 	Ev           Event
// 	EventHeader
// }

// func (ev *CompressEvent) Encode(buffer *bytes.Buffer) {
// 	EncodeUInt64Value(buffer, uint64(ev.CompressType))
// 	var buf bytes.Buffer
// 	EncodeEvent(&buf, ev.Ev)
// 	switch ev.CompressType {
// 	case NoneCompressor:
// 		EncodeUInt64Value(buffer, uint64(buf.Len()))
// 		buffer.Write(buf.Bytes())
// 	case SnappyCompressor:
// 		evbuf := make([]byte, 0)
// 		newbuf, _ := snappy.Encode(evbuf, buf.Bytes())
// 		EncodeUInt64Value(buffer, uint64(len(newbuf)))
// 		buffer.Write(newbuf)
// 	}
// 	buf.Reset()
// }
// func (ev *CompressEvent) Decode(buffer *bytes.Buffer) (err error) {
// 	ev.CompressType, err = DecodeUInt32Value(buffer)
// 	if err != nil {
// 		return
// 	}
// 	length, err := DecodeUInt32Value(buffer)
// 	if err != nil {
// 		return
// 	}
// 	switch ev.CompressType {
// 	case NoneCompressor:
// 		err, ev.Ev = DecodeEvent(buffer)
// 		return err
// 	case SnappyCompressor:
// 		b := make([]byte, 0, 0)
// 		b, err = snappy.Decode(b, buffer.Next(int(length)))
// 		if err != nil {
// 			return
// 		}
// 		tmpbuf := bytes.NewBuffer(b)
// 		err, ev.Ev = DecodeEvent(tmpbuf)
// 		tmpbuf.Reset()
// 	default:
// 		return fmt.Errorf("Not supported compress type:%d", ev.CompressType)
// 	}
// }

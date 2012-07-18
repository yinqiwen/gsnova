package event

import (
	"bytes"
	"errors"
	"snappy"
	"strconv"
	//"fmt"
)

type CompressEvent struct {
	CompressType uint32
	Ev           Event
	EventHeader
}

func (ev *CompressEvent) Encode(buffer *bytes.Buffer) {
	if ev.CompressType != COMPRESSOR_NONE && ev.CompressType != COMPRESSOR_SNAPPY {
		ev.CompressType = COMPRESSOR_NONE
	}
	EncodeUInt64Value(buffer, uint64(ev.CompressType))
	var buf bytes.Buffer
	EncodeEvent(&buf, ev.Ev)
	switch ev.CompressType {
	case COMPRESSOR_NONE:
		buffer.Write(buf.Bytes())
	case COMPRESSOR_SNAPPY:
		evbuf := make([]byte, 0)
		newbuf, _ := snappy.Encode(evbuf, buf.Bytes())
		buffer.Write(newbuf)
	}
	buf.Reset()
}
func (ev *CompressEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.CompressType, err = DecodeUInt32Value(buffer)
	if err != nil {
		return
	}
	switch ev.CompressType {
	case COMPRESSOR_NONE:
		err, ev.Ev = DecodeEvent(buffer)
		return err
	case COMPRESSOR_SNAPPY:
		b := make([]byte, 0, 0)
		b, err = snappy.Decode(b, buffer.Bytes())
		if err != nil {
			b = nil
			return
		}
		tmpbuf := bytes.NewBuffer(b)
		err, ev.Ev = DecodeEvent(tmpbuf)
		tmpbuf.Reset()
		return err
	default:
		return errors.New("Not supported compress type:" + strconv.Itoa(int(ev.CompressType)))
	}
	return nil
}

func (ev *CompressEvent) GetType() uint32 {
	return COMPRESS_EVENT_TYPE
}
func (ev *CompressEvent) GetVersion() uint32 {
	return 1
}

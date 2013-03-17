package event

import (
	"bytes"
	"code.google.com/p/snappy-go/snappy"
	"errors"
//	"github.com/bkaradzic/go-lz4"
//	"io/ioutil"
	"strconv"
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
//	case COMPRESSOR_LZ4:
//		newbuf, _ := ioutil.ReadAll(lz4.NewWriter(&buf))
//		buffer.Write(newbuf)
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
//	case COMPRESSOR_LZ4:
//		lz4r := lz4.NewReader(buffer)
//		data, _ := ioutil.ReadAll(lz4r)
//		tmpbuf := bytes.NewBuffer(data)
//		err, ev.Ev = DecodeEvent(tmpbuf)
//		tmpbuf.Reset()
//		return err
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

type CompressEventV2 struct {
	CompressType uint32
	Ev           Event
	EventHeader
}

func (ev *CompressEventV2) Encode(buffer *bytes.Buffer) {
	if ev.CompressType != COMPRESSOR_NONE && ev.CompressType != COMPRESSOR_SNAPPY {
		ev.CompressType = COMPRESSOR_NONE
	}
	EncodeUInt64Value(buffer, uint64(ev.CompressType))
	var buf bytes.Buffer
	EncodeEvent(&buf, ev.Ev)
	switch ev.CompressType {
	case COMPRESSOR_NONE:
		EncodeUInt64Value(buffer, uint64(buf.Len()))
		buffer.Write(buf.Bytes())
	case COMPRESSOR_SNAPPY:
		evbuf := make([]byte, 0)
		newbuf, _ := snappy.Encode(evbuf, buf.Bytes())
		EncodeUInt64Value(buffer, uint64(len(newbuf)))
		buffer.Write(newbuf)
//	case COMPRESSOR_LZ4:
//		newbuf, _ := ioutil.ReadAll(lz4.NewWriter(&buf))
//		EncodeUInt64Value(buffer, uint64(len(newbuf)))
//		buffer.Write(newbuf)
	}
	buf.Reset()
}
func (ev *CompressEventV2) Decode(buffer *bytes.Buffer) (err error) {
	ev.CompressType, err = DecodeUInt32Value(buffer)
	if err != nil {
		return
	}
	length, err := DecodeUInt32Value(buffer)
	if err != nil {
		return
	}
	switch ev.CompressType {
	case COMPRESSOR_NONE:
		err, ev.Ev = DecodeEvent(buffer)
		return err
	case COMPRESSOR_SNAPPY:
		b := make([]byte, 0, 0)
		b, err = snappy.Decode(b, buffer.Next(int(length)))
		if err != nil {
			b = nil
			return
		}
		tmpbuf := bytes.NewBuffer(b)
		err, ev.Ev = DecodeEvent(tmpbuf)
		tmpbuf.Reset()
		return err
//	case COMPRESSOR_LZ4:
//		lz4r := lz4.NewReader(bytes.NewBuffer(buffer.Next(int(length))))
//		data, _ := ioutil.ReadAll(lz4r)
//		tmpbuf := bytes.NewBuffer(data)
//		err, ev.Ev = DecodeEvent(tmpbuf)
//		tmpbuf.Reset()
//		return err
	default:
		return errors.New("Not supported compress type:" + strconv.Itoa(int(ev.CompressType)))
	}
	return nil
}

func (ev *CompressEventV2) GetType() uint32 {
	return COMPRESS_EVENT_TYPE
}
func (ev *CompressEventV2) GetVersion() uint32 {
	return 2
}

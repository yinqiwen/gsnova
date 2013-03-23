package event

import (
	"bytes"
	"encoding/binary"
	"errors"
	//"fmt"
	"reflect"
)

const (
	MAGIC_NUMBER uint16 = 0xCAFE
)

func EncodeInt64Value(buf *bytes.Buffer, v int64) {
	b := make([]byte, 10)
	size := binary.PutVarint(b, v)
	buf.Write(b[:size])
}

func DecodeInt64Value(buf *bytes.Buffer) (int64, error) {
	return binary.ReadVarint(buf)
}

func EncodeUInt32Value(buf *bytes.Buffer, v uint32) {
	b := make([]byte, 10)
	size := binary.PutUvarint(b, uint64(v))
	buf.Write(b[:size])
}

func EncodeUInt64Value(buf *bytes.Buffer, v uint64) {
	b := make([]byte, 10)
	size := binary.PutUvarint(b, v)
	buf.Write(b[:size])
}

func DecodeUInt64Value(buf *bytes.Buffer) (uint64, error) {
	return binary.ReadUvarint(buf)
}

func DecodeUInt32Value(buf *bytes.Buffer) (uint32, error) {
	tmp, err := binary.ReadUvarint(buf)
	return uint32(tmp), err
}

func DecodeInt32Value(buf *bytes.Buffer) (int32, error) {
	tmp, err := binary.ReadVarint(buf)
	return int32(tmp), err
}

func EncodeBoolValue(buf *bytes.Buffer, v bool) {
	if v {
		EncodeUInt64Value(buf, 1)
	} else {
		EncodeUInt64Value(buf, 0)
	}
}

func DecodeBoolValue(buf *bytes.Buffer) (v bool, err error) {
	var b byte
	if b, err = buf.ReadByte(); nil != err {
		return
	} else {
		if b == 0 {
			v = false
		} else {
			v = true
		}
	}
	return
}

func EncodeBytesValue(buf *bytes.Buffer, v []byte) {
	if nil == v {
		EncodeUInt64Value(buf, 0)
	} else {
		EncodeUInt64Value(buf, uint64(len(v)))
		buf.Write(v)
	}
}

func EncodeByteBufferValue(buf *bytes.Buffer, v *bytes.Buffer) {
	if nil == v {
		EncodeUInt64Value(buf, 0)
	} else {
		EncodeUInt64Value(buf, uint64(v.Len()))
		buf.ReadFrom(v)
		//buf.Write(v)
	}
}

func EncodeStringValue(buf *bytes.Buffer, v string) {
	EncodeUInt64Value(buf, uint64(len(v)))
	buf.Write([]byte(v))
}

func DecodeBytesValue(buf *bytes.Buffer) (b []byte, err error) {
	var size uint64
	if size, err = binary.ReadUvarint(buf); nil != err {
		return
	}
	b = make([]byte, size)
	buf.Read(b)
	return
}

func DecodeByteBufferValue(buf *bytes.Buffer, dst *bytes.Buffer) (err error) {
	var size uint64
	if size, err = binary.ReadUvarint(buf); nil != err {
		return
	}
	if buf.Len() < int(size) {
		return errors.New("No sufficient space.")
	}
	dst.Write(buf.Next(int(size)))
	return nil
}

func DecodeStringValue(buf *bytes.Buffer) (str string, err error) {
	var size uint64
	if size, err = binary.ReadUvarint(buf); nil != err {
		return
	}
	b := make([]byte, size)
	buf.Read(b)
	str = string(b)
	return
}

func encodeValue(buf *bytes.Buffer, v *reflect.Value) {
	switch v.Type().Kind() {
	case reflect.Bool:
		EncodeBoolValue(buf, v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		EncodeInt64Value(buf, v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		EncodeUInt64Value(buf, v.Uint())
	case reflect.String:
		EncodeBytesValue(buf, []byte(v.String()))
	case reflect.Array, reflect.Slice:
		if v.Type().Kind() == reflect.Slice && v.IsNil() {
			EncodeUInt64Value(buf, 0)
		} else {
			EncodeUInt64Value(buf, uint64(v.Len()))
			for i := 0; i < v.Len(); i++ {
				iv := v.Index(i)
				encodeValue(buf, &iv)
			}
		}
	case reflect.Map:
		if v.IsNil() {
			EncodeUInt64Value(buf, 0)
		} else {
			ks := v.MapKeys()
			EncodeUInt64Value(buf, uint64(len(ks)))
			for i := 0; i < len(ks); i++ {
				encodeValue(buf, &(ks[i]))
				vv := v.MapIndex(ks[i])
				encodeValue(buf, &vv)
			}
		}
	case reflect.Ptr:
		rv := reflect.Indirect(*v)
		encodeValue(buf, &rv)
	case reflect.Interface:
		rv := reflect.ValueOf(v.Interface())
		encodeValue(buf, &rv)
	case reflect.Struct:
		if m, exist := reflect.PtrTo(v.Type()).MethodByName("Encode"); exist {
			m.Func.Call([]reflect.Value{v.Addr(), reflect.ValueOf(buf)})
		} else {
			num := v.NumField()
			for i := 0; i < num; i++ {
				f := v.Field(i)
				encodeValue(buf, &f)
			}
		}
	}
}

func decodeValue(buf *bytes.Buffer, v *reflect.Value) error {
	switch v.Type().Kind() {
	case reflect.Bool:
		b, err := DecodeBoolValue(buf)
		if nil == err {
			v.SetBool(b)
		} else {
			return err
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b, err := binary.ReadVarint(buf)
		if nil == err {
			v.SetInt(b)
		} else {
			return err
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b, err := binary.ReadUvarint(buf)
		if nil == err {
			v.SetUint(b)
		} else {
			return err
		}
	case reflect.String:
		b, err := DecodeBytesValue(buf)
		if nil == err {
			v.SetString(string(b))
		} else {
			return err
		}
	case reflect.Array, reflect.Slice:
		b, err := binary.ReadUvarint(buf)
		if nil == err {
			sv := reflect.MakeSlice(v.Type(), int(b), int(b))
			for i := 0; i < int(b); i++ {
				iv := sv.Index(i)
				decodeValue(buf, &(iv))
			}
			v.Set(sv)
		} else {
			return err
		}
	case reflect.Map:
		b, err := binary.ReadUvarint(buf)
		if nil == err {
			vm := reflect.MakeMap(v.Type())
			for i := 0; i < int(b); i++ {
				kv := reflect.New(v.Type().Key()).Elem()
				vv := reflect.New(v.Type().Elem()).Elem()
				err := decodeValue(buf, &(kv))
				if nil != err {
					return err
				}
				err = decodeValue(buf, &(vv))
				if nil != err {
					return err
				}
				vm.SetMapIndex(kv, vv)
			}
			v.Set(vm)
		} else {
			return err
		}
	case reflect.Ptr:
		var err error
		if v.IsNil() {
			pv := reflect.New(v.Type().Elem())
			err = decodeValue(buf, &pv)
			v.Set(pv)
		} else {
			xv := v.Elem()
			err = decodeValue(buf, &xv)
		}
		if nil != err {
			return err
		} else {
			//v.Set(pv.Addr())
		}
	case reflect.Interface:
		rv := v.Elem()
		if rv.Type().Kind() == reflect.Interface {
			return errors.New("Loop interface:")
		}
		if !rv.CanSet() {
			return errors.New("Can not set interface")
		}
		err := decodeValue(buf, &rv)
		if nil != err {
			return err
		}
	case reflect.Struct:
		if m, exist := reflect.PtrTo(v.Type()).MethodByName("Decode"); exist {
			ret := m.Func.Call([]reflect.Value{v.Addr(), reflect.ValueOf(buf)})
			if ret[0].IsNil() {
				return nil
			}
			return ret[0].Interface().(error)
		} else {
			num := v.NumField()
			if !v.CanSet() {
				return errors.New("struct not settable")
			}
			for i := 0; i < num; i++ {
				f := v.Field(i)
				if !f.CanSet() {
					return errors.New("Field not settable")
				}
				err := decodeValue(buf, &f)
				if nil != err {
					return err
				}
			}
		}

	default:
		return errors.New("Unsupported type:" + v.Type().Name())
	}
	return nil
}

type EventHeader struct {
	Type    uint32
	Version uint32
	Hash    uint32
}

func (h *EventHeader) GetHash() uint32 {
	return h.Hash
}
func (h *EventHeader) SetHash(hash uint32) {
	h.Hash = hash
}

func (header *EventHeader) Encode(buffer *bytes.Buffer) {
	EncodeUInt64Value(buffer, uint64(header.Type))
	EncodeUInt64Value(buffer, uint64(header.Version))
	EncodeUInt64Value(buffer, uint64(header.Hash))
}
func (header *EventHeader) Decode(buffer *bytes.Buffer) error {
	var err error
	header.Type, err = DecodeUInt32Value(buffer)
	if nil != err {
		return err
	}
	header.Version, err = DecodeUInt32Value(buffer)
	if nil != err {
		return err
	}
	header.Hash, err = DecodeUInt32Value(buffer)
	if nil != err {
		return err
	}
	return nil
}

type Event interface {
	Encode(buffer *bytes.Buffer)
	Decode(buffer *bytes.Buffer) error
	GetType() uint32
	GetVersion() uint32
	GetHash() uint32
	SetHash(hash uint32)
}

func EncodeValue(buf *bytes.Buffer, ev interface{}) error {
	if exist, tk := GetRegistTypeVersion(ev); !exist {
		return errors.New("No regist info to encode value.")
	} else {
		EncodeUInt64Value(buf, uint64(tk.Type))
		EncodeUInt64Value(buf, uint64(tk.Version))
		rv := reflect.ValueOf(ev)
		encodeValue(buf, &rv)
	}
	return nil
}

func DecodeValue(buf *bytes.Buffer) (err error, ev interface{}) {
	t, err := DecodeUInt32Value(buf)
	if nil != err {
		return
	}
	version, err := DecodeUInt32Value(buf)
	if nil != err {
		return
	}
	err, ev = NewObjectInstance(t, version)
	rv := reflect.ValueOf(ev)
	err = decodeValue(buf, &rv)
	return
}

func EncodeEvent(buf *bytes.Buffer, ev Event) {
	var header EventHeader
	header.Type = ev.GetType()
	header.Version = ev.GetVersion()
	header.Hash = ev.GetHash()
	header.Encode(buf)
	ev.Encode(buf)
}

func DecodeEvent(buf *bytes.Buffer) (err error, ev Event) {
	var header EventHeader
	if err = header.Decode(buf); nil != err {
		return
	}
	var tmp interface{}
	if err, tmp = NewEventInstance(header.Type, header.Version); nil != err {
		return
	}
	ev = tmp.(Event)
	ev.SetHash(header.Hash)
	err = ev.Decode(buf)
	return
}

func ExtractEvent(ev Event) Event {
	for ev.GetType() == ENCRYPT_EVENT_TYPE || ev.GetType() == COMPRESS_EVENT_TYPE {
		encrypt, ok := ev.(*EncryptEvent)
		if ok {
			ev = encrypt.Ev
		}
		encryptv2, ok := ev.(*EncryptEventV2)
		if ok {
			ev = encryptv2.Ev
		}
		compress, ok := ev.(*CompressEvent)
		if ok {
			ev = compress.Ev
		}
		compressv2, ok := ev.(*CompressEventV2)
		if ok {
			ev = compressv2.Ev
		}
	}
	return ev
}

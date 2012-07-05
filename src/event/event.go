package event

import (
	"bytes"
	"encoding/binary"
	"reflect"
)

func encodeInt64Value(buf *bytes.Buffer, v int64) {
	b := make([]byte, 10)
	size := binary.PutVarint(b, v)
	buf.Write(b[:size])
}

func encodeUInt64Value(buf *bytes.Buffer, v uint64) {
	b := make([]byte, 10)
	size := binary.PutUvarint(b, v)
	buf.Write(b[:size])
}

//func decodeUInt64Value(buf *bytes.Buffer) (v uint64, err error){
//	return binary.ReadUvarint(buf)
//}

func encodeBoolValue(buf *bytes.Buffer, v bool) {
	if v {
		encodeUInt64Value(buf, 1)
	} else {
		encodeUInt64Value(buf, 0)
	}
}

func decodeBoolValue(buf *bytes.Buffer) (v bool, err error) {
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

func encodeBytesValue(buf *bytes.Buffer, v []byte) {
	if nil == v {
		encodeUInt64Value(buf, 0)
	} else {
		encodeUInt64Value(buf, uint64(len(v)))
		buf.Write(v)
	}
}

func decodeBytesValue(buf *bytes.Buffer) (b []byte, err error) {
	var size uint64
	if size, err = binary.ReadUvarint(buf); nil != err {
		return
	}
	b = make([]byte, size)
	buf.Read(b)
	return
}

func encodeValue(buf *bytes.Buffer, v reflect.Value) {
	switch v.Type().Kind() {
	case reflect.Bool:
		encodeBoolValue(buf, v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		encodeInt64Value(buf, v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		encodeUInt64Value(buf, v.Uint())
	case reflect.String:
		encodeBytesValue(buf, []byte(v.String()))
	case reflect.Array, reflect.Slice:
		if v.Type().Kind() == reflect.Slice && v.IsNil() {
			encodeUInt64Value(buf, 0)
		} else {
			encodeUInt64Value(buf, uint64(v.Len()))
			for i := 0; i < v.Len(); i++ {
				encodeValue(buf, v.Index(i))
			}
		}
	case reflect.Map:
		if v.IsNil() {
			encodeUInt64Value(buf, 0)
		} else {
			ks := v.MapKeys()
			encodeUInt64Value(buf, uint64(len(ks)))
			for i := 0; i < len(ks); i++ {
				encodeValue(buf, ks[i])
				encodeValue(buf, v.MapIndex(ks[i]))
			}
		}
	case reflect.Ptr:
		encodeValue(buf, reflect.Indirect(v))
	case reflect.Interface:
		encodeValue(buf, reflect.ValueOf(v.Interface()))
	case reflect.Struct:
		num := v.NumField()
		for i := 0; i < num; i++ {
			f := v.Field(i)
			encodeValue(buf, f)
		}
	}
}

func decodeValue(buf *bytes.Buffer, v reflect.Value) error {
	switch v.Type().Kind() {
	case reflect.Bool:
		b, err := decodeBoolValue(buf)
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
		b, err := decodeBytesValue(buf)
		if nil == err {
			v.SetString(string(b))
		} else {
			return err
		}
	case reflect.Array, reflect.Slice:
		b, err := binary.ReadUvarint(buf)
		if nil == err {
			sv := reflect.MakeSlice(v.Type(), int(b), int(b))
			kt := v.Type().Elem()
			var i uint64
			for i = 0; i < b; i++ {
				iv := reflect.Zero(kt)
				reflect.Append(sv, iv)
			}
			v.Set(sv)
		} else {
			return err
		}
	case reflect.Map:
		b, err := binary.ReadUvarint(buf)
		if nil == err {
			vm := reflect.MakeMap(v.Type())
			kt := v.Type().Key()
			vt := v.Type().Elem()
			var i uint64
			for i = 0; i < b; i++ {
				kv := reflect.Zero(kt)
				vv := reflect.Zero(vt)
				err := decodeValue(buf, kv)
				if nil != err {
					return err
				}
				err = decodeValue(buf, vv)
				if nil != err {
					return err
				}
				v.SetMapIndex(kv, vv)
			}
			v.Set(vm)
		} else {
			return err
		}
	case reflect.Ptr:
		pv := reflect.New(v.Type().Elem())
		err := decodeValue(buf, reflect.Indirect(pv))
		if nil != err {
			return err
		} else {
			v.Set(pv)
		}
	case reflect.Interface:
	    rv := reflect.Zero(v.Type())
		err := decodeValue(buf, rv)
		if nil != err {
			return err
		} else {
			v.Set(rv)
		}
	case reflect.Struct:
		num := v.NumField()
		for i := 0; i < num; i++ {
			f := v.Field(i)
			decodeValue(buf, f)
		}
	}
	return nil
}

func Encode(buf *bytes.Buffer, ev interface{}) {
	rv := reflect.ValueOf(ev)
	encodeValue(buf, rv)
}


func Decode(buf *bytes.Buffer, ev interface{}) error {
	rv := reflect.ValueOf(ev)
	return decodeValue(buf, rv)
}
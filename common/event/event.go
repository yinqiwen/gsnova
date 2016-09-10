package event

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rc4"
	"encoding/binary"
	"errors"
	"log"
	"math/rand"
	"reflect"
	"runtime"
	"strings"

	//"github.com/codahale/chacha20"
	"golang.org/x/crypto/salsa20"
)

const (
	largeEventLimit = 1024 * 1024
)

var EBNR = errors.New("Event buffer not ready")
var ErrToolargeEvent = errors.New("Event too large content")

var secretKey []byte
var salsa20Key [32]byte

var defaultEncryptMethod int
var aes256gcm cipher.AEAD

type EventFlags uint64

func (f EventFlags) IsSnappyEnable() bool {
	return f&SnappyCompressor > 0
}
func (f EventFlags) GetEncrytFlag() int {
	return int(f >> 4)
}
func (f *EventFlags) EnableSnappy() {
	(*f) = EventFlags(uint64(*f) | SnappyCompressor)
}

func (f *EventFlags) EnableEncrypt(v int) {
	(*f) = EventFlags(uint64(*f) | uint64(v<<4))
}

func SetDefaultSecretKey(method string, key string) {
	secretKey = []byte(key)
	if len(secretKey) > 32 {
		secretKey = secretKey[0:32]
	} else {
		tmp := make([]byte, 32)
		copy(tmp[0:len(secretKey)], secretKey)
	}
	copy(salsa20Key[:], secretKey[:32])
	aesblock, _ := aes.NewCipher(secretKey)
	aes256gcm, _ = cipher.NewGCM(aesblock)
	defaultEncryptMethod = Salsa20Encrypter
	if strings.EqualFold(method, "rc4") {
		defaultEncryptMethod = RC4Encrypter
	} else if strings.EqualFold(method, "salsa20") {
		defaultEncryptMethod = Salsa20Encrypter
	} else if strings.EqualFold(method, "aes") {
		defaultEncryptMethod = AES256Encrypter
	} else if strings.EqualFold(method, "chacha20") {
		defaultEncryptMethod = Chacha20Encrypter
	} else if strings.EqualFold(method, "none") {
		defaultEncryptMethod = 0
	} else if strings.EqualFold(method, "auto") {
		if strings.Contains(runtime.GOARCH, "386") || strings.Contains(runtime.GOARCH, "amd64") {
			defaultEncryptMethod = AES256Encrypter
		} else if strings.Contains(runtime.GOARCH, "arm") {
			defaultEncryptMethod = Chacha20Encrypter
		}
		//log.Printf("Auto select fastest encrypt method:%d", defaultEncryptMethod)
	}
}

func GetDefaultCryptoMethod() uint8 {
	return uint8(defaultEncryptMethod)
}

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

func DecodeUInt16Value(buf *bytes.Buffer) (uint16, error) {
	tmp, err := binary.ReadUvarint(buf)
	return uint16(tmp), err
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
	if size >= largeEventLimit {
		return nil, ErrToolargeEvent
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
	if size >= largeEventLimit {
		return ErrToolargeEvent
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
	if size >= largeEventLimit {
		return "", ErrToolargeEvent
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
	Type  uint16
	Id    uint32
	Flags EventFlags
}

func (h *EventHeader) GetType() uint16 {
	return h.Type
}
func (h *EventHeader) GetFlags() EventFlags {
	return h.Flags
}

func (h *EventHeader) GetId() uint32 {
	return h.Id
}
func (h *EventHeader) SetId(hash uint32) {
	h.Id = hash
}

func (header *EventHeader) Encode(buffer *bytes.Buffer) {
	EncodeUInt64Value(buffer, uint64(header.Type))
	EncodeUInt64Value(buffer, uint64(header.Id))
	EncodeUInt64Value(buffer, uint64(header.Flags))
}
func (header *EventHeader) Decode(buffer *bytes.Buffer) error {
	var err error
	header.Type, err = DecodeUInt16Value(buffer)
	if nil != err {
		return err
	}
	header.Id, err = DecodeUInt32Value(buffer)
	if nil != err {
		return err
	}
	var f uint64
	f, err = DecodeUInt64Value(buffer)
	if nil != err {
		return err
	}
	header.Flags = EventFlags(f)
	return nil
}

type Event interface {
	Encode(buffer *bytes.Buffer)
	Decode(buffer *bytes.Buffer) error
	GetId() uint32
	SetId(id uint32)
}

func EncodeValue(buf *bytes.Buffer, ev interface{}) error {
	if tk := GetRegistType(ev); 0 == tk {
		return errors.New("No regist info to encode value.")
	} else {
		EncodeUInt64Value(buf, uint64(tk))
		rv := reflect.ValueOf(ev)
		encodeValue(buf, &rv)
	}
	return nil
}

func DecodeValue(buf *bytes.Buffer) (err error, ev interface{}) {
	t, err := DecodeUInt16Value(buf)
	if nil != err {
		return
	}
	err, ev = NewObjectInstance(t)
	rv := reflect.ValueOf(ev)
	err = decodeValue(buf, &rv)
	return
}

type CryptoContext struct {
	Method    uint8
	DecryptIV uint64
	EncryptIV uint64
}

func EncryptEvent(buf *bytes.Buffer, ev Event, ctx *CryptoContext) error {
	start := buf.Len()
	buf.Write(make([]byte, 4))
	var header EventHeader
	header.Type = GetRegistType(ev)
	header.Id = ev.GetId()
	header.Flags = 0
	header.Encode(buf)
	method := ctx.Method
	ev.Encode(buf)

	elen := uint32(buf.Len() - start)
	eventContent := buf.Bytes()[start+4:]

	var nonce []byte
	encryptIV := ctx.EncryptIV
	if header.Type == EventAuth {
		method = uint8(Salsa20Encrypter)
		encryptIV = 0
		if elen > 256 {
			log.Fatalf("Too large auth event with length:%d", elen)
		}
	}
	switch method {
	case Salsa20Encrypter:
		fallthrough
	case Chacha20Encrypter:
		nonce = make([]byte, 8)
	case AES256Encrypter:
		nonce = make([]byte, 12)
		elen += uint32(aes256gcm.Overhead())
	}
	if len(nonce) > 0 {
		iv := encryptIV ^ uint64(elen)
		binary.LittleEndian.PutUint64(nonce, iv)
	}

	switch method {
	case Salsa20Encrypter:
		salsa20.XORKeyStream(eventContent, eventContent, nonce, &salsa20Key)
	case RC4Encrypter:
		rc4Cipher, _ := rc4.NewCipher(secretKey)
		rc4Cipher.XORKeyStream(eventContent, eventContent)
	case AES256Encrypter:
		bb := aes256gcm.Seal(eventContent[:0], nonce, eventContent, nil)
		if len(bb)-len(eventContent) != aes256gcm.Overhead() {
			log.Printf("Expected aes bytes %d  after encrypt %d bytes", len(bb), len(eventContent))
		}
		copy(eventContent, bb[0:len(eventContent)])
		if len(bb) > len(eventContent) {
			buf.Write(bb[len(eventContent):])
		}
	case Chacha20Encrypter:
		chacha20XOR(nonce, eventContent, eventContent)
		//chacha20Cipher, _ := chacha20.New(secretKey, nonce)
		//chacha20Cipher.XORKeyStream(eventContent, eventContent)
	}

	//log.Printf("Enc event(%d):%T with iv:%d with len:%d_%d %d", ev.GetId(), ev, encryptIV, elen, len(eventContent), method)
	if header.Type == EventAuth {
		base := rand.Int31n(0xFFFFFF)
		elen = (uint32(base) << 8) + elen
	} else {
		elen = elen ^ uint32(encryptIV)
	}
	binary.LittleEndian.PutUint32(buf.Bytes()[start:start+4], elen)
	if header.Type != EventAuth {
		ctx.EncryptIV++
	}
	return nil
}

func DecryptEvent(buf *bytes.Buffer, ctx *CryptoContext) (err error, ev Event) {
	if buf.Len() < 4 {
		return EBNR, nil
	}
	elen := binary.LittleEndian.Uint32(buf.Bytes()[0:4])
	method := ctx.Method
	if method == 0 && ctx.DecryptIV == 0 {
		method = Salsa20Encrypter
		elen = elen & uint32(0xFF)
	} else {
		elen = elen ^ uint32(ctx.DecryptIV)
	}
	if elen > uint32(buf.Len()) {
		return EBNR, nil
	}
	if elen >= largeEventLimit {
		return ErrToolargeEvent, nil
	}
	buf.Next(4)
	body := buf.Next(int(elen - 4))

	var nonce []byte
	switch method {
	case Salsa20Encrypter:
		fallthrough
	case Chacha20Encrypter:
		nonce = make([]byte, 8)
	case AES256Encrypter:
		nonce = make([]byte, 12)
	}
	if len(nonce) > 0 {
		iv := ctx.DecryptIV ^ uint64(elen)
		binary.LittleEndian.PutUint64(nonce, iv)
	}

	switch method {
	case Salsa20Encrypter:
		salsa20.XORKeyStream(body, body, nonce, &salsa20Key)
	case RC4Encrypter:
		rc4Cipher, _ := rc4.NewCipher(secretKey)
		rc4Cipher.XORKeyStream(body, body)
	case AES256Encrypter:
		bb, err := aes256gcm.Open(body[:0], nonce, body, nil)
		if nil != err {
			return err, nil
		}
		body = bb
	case Chacha20Encrypter:
		chacha20XOR(nonce, body, body)
		//cipher, _ := chacha20.New(secretKey, nonce)
		//cipher.XORKeyStream(body, body)
	}
	ebuf := bytes.NewBuffer(body)
	var header EventHeader
	if err = header.Decode(ebuf); nil != err {
		log.Printf("Failed to decode event header")
		return
	}
	//log.Printf("Dec event(%d) with iv:%d with len:%d_%d  %d  %d", header.Id, ctx.DecryptIV, elen, len(body), method, header.Type)
	var tmp interface{}
	if err, tmp = NewEventInstance(header.Type); nil != err {
		log.Printf("Failed to decode event with err:%v with len:%d", err, elen)
		return
	}
	ev = tmp.(Event)
	ev.SetId(header.Id)
	err = ev.Decode(ebuf)
	if nil != err {
		log.Printf("Failed to decode event:%T with err:%v with len:%d", tmp, err, elen)
	}
	if header.Type != EventAuth {
		ctx.DecryptIV++
	}
	return
}

package se

import (
	"bytes"
)

func Encrypt(buffer *bytes.Buffer) *bytes.Buffer {
	buf := buffer.Bytes()
	for i := 0; i < len(buf); i++ {
		var k int32 = int32(buf[i])
		k -= 1
		if k < 0 {
			k += 256	
		}
		buf[i] = uint8(k)
	}
	return bytes.NewBuffer(buf)
}

func Decrypt(buffer *bytes.Buffer) *bytes.Buffer {
	buf := buffer.Bytes()
	for i := 0; i < len(buf); i++ {
		var k int32 = int32(buf[i])
		k += 1
		if k > 256 {
			k -= 256
		}
		buf[i] = uint8(k)
	}
	return bytes.NewBuffer(buf)
}

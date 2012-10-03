package codec

import (
	"bytes"
)


func WriteVarString(buffer *bytes.Buffer, str string) {
	WriteUvarint(buffer, uint64(len(str)))
	buffer.WriteString(str)
}
func WriteVarBytes(buffer *bytes.Buffer, str []byte) {
	WriteUvarint(buffer, uint64(len(str)))
	buffer.Write(str)
}
func ReadVarString(buffer *bytes.Buffer) (line string, ok bool) {
	length, err := ReadUvarint(buffer)
	if err != nil {
		ok = false
		return
	}
	buf := make([]byte, length)
	realLen, err := buffer.Read(buf)
	if err != nil || uint64(realLen) < length {
		ok = false
		return
	}
	line = string(buf)
	ok = true
	return
}
func ReadVarBytes(buffer *bytes.Buffer) (line []byte, ok bool) {
	length, err := ReadUvarint(buffer)
	if err != nil {
		ok = false
		return
	}
	if length >0 {
	   buf := make([]byte, length)
	   realLen, err := buffer.Read(buf)
	   if err != nil || uint64(realLen) < length {
		  ok = false
		  return
	   }
	   line = buf
	}
	ok = true
	return
}
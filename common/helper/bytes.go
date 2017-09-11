package helper

import (
	"bytes"
	"crypto/aes"
)

func PKCS7Pad(buf *bytes.Buffer, blen int) {
	padding := 16 - (blen % 16)
	for i := 0; i < padding; i++ {
		buf.WriteByte(byte(padding))
	}
}

// Returns slice of the original data without padding.
func PKCS7Unpad(in []byte) []byte {
	if len(in) == 0 {
		return nil
	}

	padding := in[len(in)-1]
	if int(padding) > len(in) || padding > aes.BlockSize {
		return nil
	} else if padding == 0 {
		return nil
	}

	for i := len(in) - 1; i > len(in)-int(padding)-1; i-- {
		if in[i] != padding {
			return nil
		}
	}
	return in[:len(in)-int(padding)]
}

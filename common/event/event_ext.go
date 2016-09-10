// +build !appengine

package event

import "github.com/codahale/chacha20"

func chacha20XOR(nonce []byte, dst []byte, src []byte) {
	chacha20Cipher, _ := chacha20.New(secretKey, nonce)
	chacha20Cipher.XORKeyStream(dst, src)
}

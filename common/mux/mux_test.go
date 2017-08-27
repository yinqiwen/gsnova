package mux

import (
	"bytes"
	"log"
	"testing"
)

type A struct {
	VV int
}

func TestXYZ(t *testing.T) {
	var buffer bytes.Buffer
	req := &AuthRequest{}
	req.CipherCounter = 121
	req.CipherMethod = "asasas"
	req.User = "asdasdas"
	//r := rand.New(rand.NewSource(time.Now().UnixNano()))
	//req.Rand = []byte(helper.RandAsciiString(int(r.Int31n(128))))
	WriteMessage(&buffer, req)

	log.Printf("#### %d", buffer.Len())

	zz := &AuthRequest{}
	err := ReadMessage(&buffer, zz)
	log.Printf("#### %v %v", zz, err)

	// req := &A{}
	// req.VV = 121
	// WriteMessage(&buffer, req)

	// log.Printf("#### %d", buffer.Len())

	// zz := &A{}
	// err := ReadMessage(&buffer, zz)
	// log.Printf("#### %v %v", zz, err)
}

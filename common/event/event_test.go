package event

import (
	"bytes"
	//	"reflect"
	"net/http"
	"testing"
	//"time"
	"fmt"
)

func benchamark(n int) {
	var request HTTPRequestEvent
	request.Headers = make(http.Header)
	request.Headers.Add("A", "CC")
	request.Headers.Add("D", "ZZ")
	request.URL = "hello"
	request.Method = "GET"
	var buf bytes.Buffer
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&buf, "hello, world, ##########11111111%d\n", i)
	}
	request.Content = buf.Bytes()
	ctx := CryptoContext{}
	ctx.Method = GetDefaultCryptoMethod()
	iv := uint64(101)
	ctx.EncryptIV = iv
	ctx.DecryptIV = iv
	for i := 0; i < n; i++ {
		var buf bytes.Buffer
		EncryptEvent(&buf, &request, &ctx)
		err, _ := DecryptEvent(&buf, &ctx)
		if nil != err {
			fmt.Printf("###%v", err)
			return
		}
	}
	// var buf bytes.Buffer
	// EncryptEvent(&buf, &request, 0)

	// //var cmp HTTPRequestEvent
	// err, tmp := DecryptEvent(&buf, 0)
	// fmt.Printf("%v\n", err)
	// cmp, _ := tmp.(*HTTPRequestEvent)
	// fmt.Printf("%s %s %s %v\n", cmp.URL, cmp.Method, string(cmp.Content), cmp.Headers)
}

func BenchmarkRC4(b *testing.B) {
	SetDefaultSecretKey("rc4", "AAAAAAasdadasfafasasdasfasasgagaga")
	benchamark(b.N)
}
func BenchmarkChacha20(b *testing.B) {
	SetDefaultSecretKey("chacha20", "AAAAAAasdadasfafasasdasfasasgagaga")
	benchamark(b.N)
}
func BenchmarkSalsa20(b *testing.B) {
	SetDefaultSecretKey("salsa20", "AAAAAAasdadasfafasasdasfasasgagaga")
	benchamark(b.N)
}

func BenchmarkAES(b *testing.B) {
	SetDefaultSecretKey("aes", "AAAAAAasdadasfafasasdasfasasgagaga")
	benchamark(b.N)
}

// func BenchmarkBlowfish(b *testing.B) {
// 	SetDefaultSecretKey("blowfish", "AAAAAAasdadasfafasasdasfasasgagaga")
// 	benchamark(b.N)
// }

type XT struct {
	X int
}

//func (x *XT)Encode(buf *bytes.Buffer){
//   //x.X = 48100
//   fmt.Println("Encode invoked!")
//}
//
//func (x *XT)Decode(buf *bytes.Buffer)error{
//    x.X = 48100
//    fmt.Println("Decode invoked!")
//    return nil
//}
// type ST struct {
// 	X int
// 	Y string
// 	Z []string
// 	M map[string]int
// 	T XT
// }

// func TestXYZ(t *testing.T) {
// 	var s ST
// 	s.Z = make([]string, 2)
// 	s.Z[0] = "hello"
// 	s.Z[1] = "world"
// 	s.X = 100
// 	s.Y = "wqy"
// 	s.M = make(map[string]int)
// 	s.M["wqy"] = 101
// 	s.T.X = 1001
// 	header := EventHeader{101, 201, 301}
// 	RegistObject(header.Type, header.Version, &s)
// 	var buf bytes.Buffer
// 	err := EncodeValue(&buf, &s)
// 	if nil != err {
// 		t.Error(err.Error())
// 		return
// 	}
// 	//cmp = nil
// 	err, dv := DecodeValue(&buf)
// 	if nil != err {
// 		t.Error(err.Error())
// 		return
// 	}
// 	cmp := dv.(*ST)
// 	if cmp.X != s.X {
// 		t.Error("X is not equal")
// 	}
// 	if cmp.Y != s.Y {
// 		t.Error("Y is not equal")
// 	}
// 	if cmp.Z[0] != s.Z[0] {
// 		t.Error("Z[0] is not equal", cmp.Z[0], s.Z[0], len(cmp.Z))
// 	}
// 	if cmp.Z[1] != s.Z[1] {
// 		t.Error("Z[1] is not equal", cmp.Z[1], s.Z[1], len(cmp.Z))
// 	}
// 	if cmp.M["wqy"] != 101 {
// 		t.Error("M[\"wqy\"] is not equal", cmp.Z[1], s.Z[1], len(cmp.Z))
// 	}
// 	if cmp.T.X != s.T.X {
// 		t.Error("T.X is not equal", cmp.Z[1], s.Z[1], len(cmp.Z))
// 	}

// 	//	var tmp bytes.Buffer
// 	//	EncodeValue(&tmp, cmp)
// 	//	err, h, dev := DecodeEvent(&tmp)
// 	//	if nil != err {
// 	//		t.Error(err.Error())
// 	//		return
// 	//	}
// 	//	if *h != header {
// 	//		t.Error("header not equal")
// 	//		return
// 	//	}
// 	//
// 	//	if (dev.(*ST)).Y != cmp.Y {
// 	//		t.Error("event not equal" + cmp.Y)
// 	//		return
// 	//	}
// 	start := time.Now().UnixNano()
// 	loopcount := 1000000
// 	for i := 0; i < loopcount; i++ {
// 		var tbuf bytes.Buffer
// 		EncodeValue(&tbuf, cmp)
// 		DecodeValue(&tbuf)
// 	}
// 	end := time.Now().UnixNano()
// 	t.Errorf("Cost %dns to loop %d to encode&decode", (end - start), loopcount)
// }

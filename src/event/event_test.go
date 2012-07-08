package event

import (
	"bytes"
	//	"fmt"
	//	"reflect"
	"testing"
	"time"
)

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
type ST struct {
	X int
	Y string
	Z []string
	M map[string]int
	T XT
}

func TestXYZ(t *testing.T) {
	var s ST
	s.Z = make([]string, 2)
	s.Z[0] = "hello"
	s.Z[1] = "world"
	s.X = 100
	s.Y = "wqy"
	s.M = make(map[string]int)
	s.M["wqy"] = 101
	s.T.X = 1001
	header := EventHeader{101, 201, 301}
	RegistObject(header.Type, header.Version, &s)
	var buf bytes.Buffer
	err := EncodeValue(&buf, &s)
	if nil != err {
		t.Error(err.Error())
		return
	}
	//cmp = nil
	err, dv := DecodeValue(&buf)
	if nil != err {
		t.Error(err.Error())
		return
	}
	cmp := dv.(*ST)
	if cmp.X != s.X {
		t.Error("X is not equal")
	}
	if cmp.Y != s.Y {
		t.Error("Y is not equal")
	}
	if cmp.Z[0] != s.Z[0] {
		t.Error("Z[0] is not equal", cmp.Z[0], s.Z[0], len(cmp.Z))
	}
	if cmp.Z[1] != s.Z[1] {
		t.Error("Z[1] is not equal", cmp.Z[1], s.Z[1], len(cmp.Z))
	}
	if cmp.M["wqy"] != 101 {
		t.Error("M[\"wqy\"] is not equal", cmp.Z[1], s.Z[1], len(cmp.Z))
	}
	if cmp.T.X != s.T.X {
		t.Error("T.X is not equal", cmp.Z[1], s.Z[1], len(cmp.Z))
	}

	//	var tmp bytes.Buffer
	//	EncodeValue(&tmp, cmp)
	//	err, h, dev := DecodeEvent(&tmp)
	//	if nil != err {
	//		t.Error(err.Error())
	//		return
	//	}
	//	if *h != header {
	//		t.Error("header not equal")
	//		return
	//	}
	//
	//	if (dev.(*ST)).Y != cmp.Y {
	//		t.Error("event not equal" + cmp.Y)
	//		return
	//	}
	start := time.Now().UnixNano()
	loopcount := 1000000
	for i := 0; i < loopcount; i++ {
		var tbuf bytes.Buffer
		EncodeValue(&tbuf, cmp)
		DecodeValue(&tbuf)
	}
	end := time.Now().UnixNano()
	t.Errorf("Cost %dns to loop %d to encode&decode", (end - start), loopcount)

}

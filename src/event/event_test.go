package event

import (
	"bytes"
	"testing"
)

type ST struct {
	X int
	Y string
	Z []string
	M map[string]int
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
	var buf bytes.Buffer
	Encode(&buf, &s)

	var cmp ST
	//cmp = nil
	err := Decode(&buf, &cmp)
	if nil != err {
		t.Error(err.Error())
		return
	}

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
	
}

package event

import (
    "testing"
    "bytes"
)

type ST struct{
   X int
   Y string
   Z []string
}

func TestXYZ(t *testing.T) {
   var s ST
   s.Z = make([]string, 2)
   s.Z[0] = "hello"
   s.Z[1] = "world"
   s.X = 100;
   s.Y = "wqy"
   var buf bytes.Buffer
   Encode(&buf, &s)
   
   var cmp ST
   err := Decode(&buf, &cmp)
   if nil != err{
      t.Error(err.Error())
   }
}


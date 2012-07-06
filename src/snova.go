package main

import "fmt"
import "reflect"

type XX struct {
	X string
	a int
	z float32
}

func (x *XX)Encode(){
   //x.X = 48100
   fmt.Println("Encode invoked!")
}

func (x *XX)Decode()error{
    //x.X = 48100
    fmt.Println("Decode invoked!")
    return nil
}

type KeyPair struct {
	First uint32
	Second uint32
}

func ModStruct(s interface{}){
  v := s.(XX)
  v.X = "123"
}

func TTTT(v interface{}){

    fmt.Println("####", reflect.ValueOf(v).Type().Name(), reflect.ValueOf(v).Type().Kind())
    if reflect.ValueOf(v).Type().Kind() == reflect.Struct{
       //t := reflect.TypeOf(v)
       xv := reflect.ValueOf(&v).Elem()
       fmt.Println("@@@@@@", xv.CanSet())
       //fmt.Println("$$$$", reflect.ValueOf(pv).Type().Name(), reflect.ValueOf(pv).Type().Kind())
    }
}

func main() {
    var f, sf KeyPair
    //var xtm map[KeyPair]string
    xtm := make(map[KeyPair]string)
	fmt.Println("hellow, world")
	var x XX
	f.First = 1
	f.Second = 101
	sf = f
	xtm[f] = "1"
	fmt.Println("@@@@" + xtm[sf])
	
	rv := reflect.ValueOf(&x).Elem()
	num := rv.NumField()
	fmt.Println("Filed num:%d", num)
	for i := 0; i < num; i++ {
		f := rv.Field(i)
		fmt.Println("Filed type:", rv.Type().Field(i).Name, f.Type().Name())
	}
	s := "[" + "hello" + "]"
	fmt.Println(s)
	ModStruct(x)
	fmt.Println(x.X)
	var v reflect.Value
	fmt.Print(reflect.TypeOf(v).Kind())
	fmt.Println(reflect.ValueOf(x).CanSet())
	fmt.Println(reflect.ValueOf(&x).Elem().CanSet())
	TTTT(x)
	TTTT(&x)
	
	//method, exist := reflect.TypeOf(&x).MethodByName("Encode")
	method := reflect.ValueOf(&x).MethodByName("Encode")
	method.Call([]reflect.Value{})
//	if exist{
//	   method.Func.Call([]reflect.Value{reflect.ValueOf(&x)})
//	}else{
//	   fmt.Println("Not exist")
//	}
}

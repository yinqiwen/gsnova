package main

import "fmt"
import "reflect"

type XX struct {
	X string
	a int
	z float32
}

func ModStruct(s *XX){
  s.X = "123"
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
	fmt.Println("hellow, world")
	var x XX
	rv := reflect.ValueOf(&x).Elem()
	num := rv.NumField()
	fmt.Println("Filed num:%d", num)
	for i := 0; i < num; i++ {
		f := rv.Field(i)
		fmt.Println("Filed type:", rv.Type().Field(i).Name, f.Type().Name())
	}
	s := "[" + "hello" + "]"
	fmt.Println(s)
	ModStruct(&x)
	fmt.Println(x.X)
	var v reflect.Value
	fmt.Print(reflect.TypeOf(v).Kind())
	fmt.Println(reflect.ValueOf(x).CanSet())
	fmt.Println(reflect.ValueOf(&x).Elem().CanSet())
	TTTT(x)
	TTTT(&x)
}

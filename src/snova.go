package main

import "fmt"
import "reflect"

type XX struct {
	x string
	a int
	z float32
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
}

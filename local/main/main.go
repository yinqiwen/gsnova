package main

import (
	"flag"
	"fmt"

	//"log"
	//"net"
	"os"
	"path/filepath"
	//"time"

	"github.com/yinqiwen/gsnova/local/gsnova"
)

func main() {
	path, err := filepath.Abs(os.Args[0])
	if nil != err {
		fmt.Println(err)
		return
	}
	home, _ := filepath.Split(path)
	dir := flag.String("dir", home, "Specify running dir for gsnova")
	flag.Parse()

	err = gsnova.StartLocalProxy(*dir, nil)
	if nil != err {
		fmt.Printf("Start gsnova error:%v", err)
	} else {
		ch := make(chan int)
		<-ch
	}
}

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/common-nighthawk/go-figure"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/local"
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

	myFigure := figure.NewFigure(fmt.Sprintf("GSNOVA CLIENT %s", local.Version), "", true)
	myFigure.Print()

	err = gsnova.StartLocalProxy(*dir, nil)
	if nil != err {
		logger.Error("Start gsnova error:%v", err)
	} else {
		ch := make(chan int)
		<-ch
	}
}

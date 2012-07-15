package main

import (
	"common"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	path, err := filepath.Abs(os.Args[0])
	if nil != err {
		fmt.Println(err)
		return
	}
	common.Home, _ = filepath.Split(path)
	common.InitLogger()
	common.InitConfig()
	log.Println("=============Start GSnova " + common.Version + "=============")
	addr, exist := common.Cfg.GetProperty("LocalServer", "Listen")
	if !exist {
	   log.Fatalln("No config [LocalServer]->Listen found");
	}
	startLocalProxyServer(addr)
}

package main

import (
	"common"
	"event"
	"fmt"
	"log"
	"os"
	"paas"
	"path/filepath"
	//"util"
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
	event.Init()
	paas.InitSpac()
	var gae paas.GAE
	var google paas.Google
	err = gae.Init()
	if nil != err {
		fmt.Printf("Failed to init GAE:%s\n", err.Error())
		return
	}
	google.Init()
	common.LoadRootCA()
	log.Println("=============Start GSnova " + common.Version + "=============")
	addr, exist := common.Cfg.GetProperty("LocalServer", "Listen")
	if !exist {
		log.Fatalln("No config [LocalServer]->Listen found")
	}
	startLocalProxyServer(addr)
}

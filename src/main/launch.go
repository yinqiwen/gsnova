package main

import (
	"common"
	"event"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"proxy"
	"remote"
	"runtime"
	"time"
	"util"
)

func main() {
	path, err := filepath.Abs(os.Args[0])
	if nil != err {
		fmt.Println(err)
		return
	}
	as_server := flag.Bool("server", false, "Run as remote proxy server")
	event.Init()
	flag.Parse()
	if *as_server {
		remote.LaunchC4HttpServer()
		return
	}
	common.Home, _ = filepath.Split(path)
	common.InitLogger()
	common.InitConfig()
	proxy.InitSpac()
	proxy.InitGoogle()
	proxy.InitHosts()

	var gae proxy.GAE
	var c4 proxy.C4
	err = gae.Init()
	if nil != err {
		log.Printf("[WARN]Failed to init GAE:%s\n", err.Error())
		//return
	} else {
		//init fake cert if GAE inited success
		common.LoadRootCA()
	}
	err = c4.Init()
	if nil != err {
		log.Printf("[WARN]Failed to init C4:%s\n", err.Error())
		//return
	}

	go proxy.InitSSH()
	proxy.InitSelfWebServer()
	proxy.PostInitSpac()

	log.Printf("=============Start %s %s==============\n", common.Product, common.Version)
	addr, exist := common.Cfg.GetProperty("LocalServer", "Listen")
	if !exist {
		log.Fatalln("No config [LocalServer]->Listen found")
	}
	runtime.GOMAXPROCS(runtime.NumCPU())
	if v, exist := common.Cfg.GetBoolProperty("LocalServer", "AutoOpenWebUI"); !exist || v {
		go func() {
			time.Sleep(1 * time.Second)
			util.OpenBrowser("http://localhost:" + common.ProxyPort + "/")
		}()
	}

	startLocalProxyServer(addr)
}

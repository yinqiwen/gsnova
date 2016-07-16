package main

import (
	"flag"
	"fmt"

	"github.com/yinqiwen/gsnova/common/logger"
	//"log"
	//"net"
	"os"
	"path/filepath"
	//"time"
	"encoding/json"
	"io/ioutil"

	"math/rand"

	_ "github.com/yinqiwen/gsnova/local/handler/direct"
	_ "github.com/yinqiwen/gsnova/local/handler/gae"
	_ "github.com/yinqiwen/gsnova/local/handler/paas"
	"github.com/yinqiwen/gsnova/local/proxy"
)

func main() {
	path, err := filepath.Abs(os.Args[0])
	if nil != err {
		fmt.Println(err)
		return
	}
	home, _ := filepath.Split(path)
	conf := flag.String("file", home+"/gsnova.json", "Specify config file for gsnova")
	flag.Parse()

	data, _ := ioutil.ReadFile(*conf)
	err = json.Unmarshal(data, &proxy.GConf)
	if nil != err {
		fmt.Printf("Failed to unmarshal json:%s to config for reason:%v", data, err)
		return
	}
	if proxy.GConf.User == "" {
		b := make([]byte, 40)
		rand.Read(b)
		proxy.GConf.User = string(b)
	}
	logger.InitLogger(proxy.GConf.Log)
	proxy.Init()
}

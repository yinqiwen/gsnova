package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/yinqiwen/gsnova/common/logger"
	_ "github.com/yinqiwen/gsnova/local/gsnova"
	"github.com/yinqiwen/gsnova/local/proxy"
)

func printASCIILogo() {

	logo := `
	        ___          ___          ___          ___         ___          ___     
	       /\  \        /\  \        /\__\        /\  \       /\__\        /\  \    
	      /::\  \      /::\  \      /::|  |      /::\  \     /:/  /       /::\  \   
	     /:/\:\  \    /:/\ \  \    /:|:|  |     /:/\:\  \   /:/  /       /:/\:\  \  
	    /:/  \:\  \  _\:\~\ \  \  /:/|:|  |__  /:/  \:\  \ /:/__/  ___  /::\~\:\  \ 
	   /:/__/_\:\__\/\ \:\ \ \__\/:/ |:| /\__\/:/__/ \:\__\|:|  | /\__\/:/\:\ \:\__\
	   \:\  /\ \/__/\:\ \:\ \/__/\/__|:|/:/  /\:\  \ /:/  /|:|  |/:/  /\/__\:\/:/  /
	    \:\ \:\__\   \:\ \:\__\      |:/:/  /  \:\  /:/  / |:|__/:/  /      \::/  / 
	     \:\/:/  /    \:\/:/  /      |::/  /    \:\/:/  /   \::::/__/       /:/  /  
	      \::/  /      \::/  /       /:/  /      \::/  /     ~~~~          /:/  /   
	       \/__/        \/__/        \/__/        \/__/                    \/__/    
	`
	fmt.Println(logo)
}

func main() {
	path, err := filepath.Abs(os.Args[0])
	if nil != err {
		fmt.Println(err)
		return
	}
	home, _ := filepath.Split(path)
	conf := flag.String("conf", "./client.json", "Config file of gsnova client.")
	hosts := flag.String("hosts", "./hosts.json", "Hosts file of gsnova client.")
	pid := flag.String("pid", ".gsnova.pid", "PID file")
	cmd := flag.Bool("cmd", false, "Launch gsnova client by command line without config file.")
	listen := flag.String("listen", ":48100", "Local listen address")
	remote := flag.String("remote", "wss://xxx.paas.com", "Remote server to connect, accept schema with tcp/tls/http/https/ws/wss/http2/quic/kcp/ssh")
	user := flag.String("user", "gsnova", "Username for remote server to authorize.")
	key := flag.String("key", "809240d3a021449f6e67aa73221d42df942a308a", "Cipher key for transmission between local&remote.")
	log := flag.String("log", "color,gsnova.log", "Log file setting")
	cnip := flag.String("cnip", "./cnipset.txt", "China IP list.")
	flag.Parse()

	printASCIILogo()

	options := proxy.ProxyOptions{
		Home:  home,
		Hosts: *hosts,
		CNIP:  *cnip,
	}
	if *cmd {
		proxy.GConf.Cipher.Key = *key
		proxy.GConf.Cipher.Method = "auto"
		proxy.GConf.User = *user
		proxy.GConf.Log = strings.Split(*log, ",")
		local := proxy.ProxyConfig{}
		local.Local = *listen
		local.PAC = []proxy.PACConfig{{Remote: "default"}}
		channel := proxy.ProxyChannelConfig{}
		channel.Enable = true
		channel.Name = "default"
		channel.ConnsPerServer = 3
		channel.ServerList = []string{*remote}
		proxy.GConf.Proxy = []proxy.ProxyConfig{local}
		proxy.GConf.Channel = []proxy.ProxyChannelConfig{channel}
		options.WatchConf = false
		err = proxy.Start(options)
	} else {
		options.WatchConf = true
		options.Config = *conf
		err = proxy.Start(options)
	}
	if nil != err {
		logger.Error("Start gsnova error:%v", err)
	} else {
		if len(*pid) > 0 {
			ioutil.WriteFile(*pid, []byte(fmt.Sprintf("%d", os.Getpid())), os.ModePerm)
		}
		ch := make(chan int)
		<-ch
	}
}

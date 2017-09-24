package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/local/gsnova"
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

	// fmt.Println(" .----------------.  .----------------.  .-----------------. .----------------.  .----------------.  .----------------. ")
	// fmt.Println("| .--------------. || .--------------. || .--------------. || .--------------. || .--------------. || .--------------. |")
	// fmt.Println("| |    ______    | || |    _______   | || | ____  _____  | || |     ____     | || | ____   ____  | || |      __      | |")
	// fmt.Println("| |  .' ___  |   | || |   /  ___  |  | || ||_   \\|_   _| | || |   .'    `.   | || ||_  _| |_  _| | || |     /  \\     | |")
	// fmt.Println("| | / .'   \\_|   | || |  |  (__ \\_|  | || |  |   \\ | |   | || |  /  .--.  \\  | || |  \\ \\   / /   | || |    / /\\ \\    | |")
	// fmt.Println("| | | |    ____  | || |   '.___`-.   | || |  | |\\ \\| |   | || |  | |    | |  | || |   \\ \\ / /    | || |   / ____ \\   | |")
	// fmt.Println("| | \\ `.___]  _| | || |  |`\\____) |  | || | _| |_\\   |_  | || |  \\  `--'  /  | || |    \\ ' /     | || | _/ /    \\ \\_ | |")
	// fmt.Println("| |  `._____.'   | || |  |_______.'  | || ||_____|\\____| | || |   `.____.'   | || |     \\_/      | || ||____|  |____|| |")
	// fmt.Println("| |              | || |              | || |              | || |              | || |              | || |              | |")
	// fmt.Println("| '--------------' || '--------------' || '--------------' || '--------------' || '--------------' || '--------------' |")
	// fmt.Println(" '----------------'  '----------------'  '----------------'  '----------------'  '----------------'  '----------------' ")
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
	flag.Parse()

	printASCIILogo()

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
		err = proxy.StartProxy()
	} else {
		err = gsnova.StartLocalProxy(home, *conf, *hosts, true)
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

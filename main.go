package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/yinqiwen/gotoolkit/ots"
	"github.com/yinqiwen/gsnova/common/channel"
	_ "github.com/yinqiwen/gsnova/common/channel/common"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/local"
	"github.com/yinqiwen/gsnova/remote"
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
	// if err := agent.Listen(agent.Options{}); err != nil {
	// 	log.Fatal(err)
	// }
	path, err := filepath.Abs(os.Args[0])
	if nil != err {
		fmt.Println(err)
		return
	}
	//common options
	admin := flag.String("admin", "", "Admin listen address")
	version := flag.Bool("version", false, "Print version.")
	cmd := flag.Bool("cmd", false, "Launch gsnova  by command line without config file.")
	isClient := flag.Bool("client", false, "Launch gsnova as client.")
	isServer := flag.Bool("server", false, "Launch gsnova as server.")
	pid := flag.String("pid", ".gsnova.pid", "PID file")
	conf := flag.String("conf", "", "Config file of gsnova.")
	key := flag.String("key", "809240d3a021449f6e67aa73221d42df942a308a", "Cipher key for transmission between local&remote.")
	log := flag.String("log", "color,gsnova.log", "Log file setting")
	window := flag.String("window", "", "Max mux stream window size, default 512K")
	windowRefresh := flag.String("window_refresh", "", "Mux stream window refresh size, default 32K")
	pingInterval := flag.Int("ping_interval", 30, "Channel ping interval seconds.")
	user := flag.String("user", "gsnova", "Username for remote server to authorize.")

	//client options
	cnip := flag.String("cnip", "./cnipset.txt", "China IP list.")
	var hops channel.HopServers
	home, _ := filepath.Split(path)
	hosts := flag.String("hosts", "./hosts.json", "Hosts file of gsnova client.")
	listen := flag.String("listen", ":48100", "Local client listen address")
	flag.Var(&hops, "hop", "Next proxy hop server to connect for client, eg:wss://xxx.paas.com")

	//server options
	httpServer := flag.String("http", "", "Remote HTTP/Websocket proxy server listen address")
	http2Server := flag.String("http2", "", "Remote HTTP2 proxy server listen address")
	tcpServer := flag.String("tcp", "", "Remote TCP proxy server listen address")
	quicServer := flag.String("quic", "", "Remote QUIC proxy server listen address")
	kcpServer := flag.String("kcp", "", "Remote KCP proxy server listen address")
	tlsServer := flag.String("tls", "", "Remote TLS proxy server listen address")

	flag.Parse()

	if *version {
		fmt.Printf("GSnova version:%s\n", channel.Version)
		return
	}

	printASCIILogo()

	confile := *conf
	runAsClient := false
	if !(*isServer) && !(*isClient) {
		runAsClient = true
	} else if *isClient != *isServer {
		runAsClient = *isClient
	} else {
		logger.Error("GSnova can not run both as client & server.")
		return
	}

	if len(*admin) > 0 {
		err := ots.StartTroubleShootingServer(*admin)
		if nil != err {
			logger.Error("Failed to start admin server with reason:%v", err)
		}
	}

	if runAsClient {
		options := local.ProxyOptions{
			Home:  home,
			Hosts: *hosts,
			CNIP:  *cnip,
		}
		if *cmd {
			if len(hops) == 0 {
				logger.Error("At least one -hop argument required.", err)
				flag.PrintDefaults()
				return
			}
			local.GConf.Mux.MaxStreamWindow = *window
			local.GConf.Mux.StreamMinRefresh = *windowRefresh
			local.GConf.Cipher.Key = *key
			local.GConf.Cipher.Method = "auto"
			local.GConf.Cipher.User = *user
			local.GConf.Log = strings.Split(*log, ",")
			proxyConf := local.ProxyConfig{}
			proxyConf.Local = *listen
			proxyConf.PAC = []local.PACConfig{{Remote: "default"}}
			ch := channel.ProxyChannelConfig{}
			ch.Enable = true
			ch.Name = "default"
			ch.ConnsPerServer = 3
			ch.HeartBeatPeriod = *pingInterval
			ch.ServerList = []string{hops[0]}
			ch.Hops = hops[1:]
			local.GConf.Proxy = []local.ProxyConfig{proxyConf}
			local.GConf.Channel = []channel.ProxyChannelConfig{ch}
			options.WatchConf = false
			err = local.Start(options)
		} else {
			if len(confile) == 0 {
				confile = "./client.json"
			}
			options.WatchConf = true
			options.Config = confile
			err = local.Start(options)
		}
		if nil != err {
			logger.Error("Start gsnova error:%v", err)
		}
	} else {
		//run as server
		remote.InitDefaultConf()
		if !(*cmd) {
			if len(confile) == 0 {
				confile = "./server.json"
			}
			if _, err := os.Stat(confile); nil == err {
				logger.Info("Load server conf from file:%s", confile)
				data, err := helper.ReadWithoutComment(confile, "//")

				//data, err := ioutil.ReadFile(file)
				if nil == err {
					err = json.Unmarshal(data, &remote.ServerConf)
				}
				if nil != err {
					logger.Error("Failed to load server config:%s for reason:%v", confile, err)
					return
				}
			}
		}
		if *cmd {
			if len(*httpServer) > 0 {
				remote.ServerConf.HTTP.Listen = *httpServer
			}
			if len(*tcpServer) > 0 {
				remote.ServerConf.TCP.Listen = *tcpServer
			}
			if len(*quicServer) > 0 {
				remote.ServerConf.QUIC.Listen = *quicServer
			}
			if len(*kcpServer) > 0 {
				remote.ServerConf.KCP.Listen = *kcpServer
			}
			if len(*http2Server) > 0 {
				remote.ServerConf.HTTP2.Listen = *http2Server
			}
			if len(*tlsServer) > 0 {
				remote.ServerConf.TLS.Listen = *tlsServer
			}
			if len(*key) > 0 {
				remote.ServerConf.Cipher.Key = *key
			}
			if len(*user) > 0 {
				remote.ServerConf.Cipher.User = *user
			}
			if len(*log) > 0 {
				remote.ServerConf.Log = strings.Split(*log, ",")
			}
			if len(*window) > 0 {
				remote.ServerConf.Mux.MaxStreamWindow = *window
			}
			if len(*windowRefresh) > 0 {
				remote.ServerConf.Mux.StreamMinRefresh = *windowRefresh
			}
		}
		if len(remote.ServerConf.KCP.Listen) > 0 {
			config := &remote.ServerConf.KCP.Params
			switch config.Mode {
			case "normal":
				config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 0, 40, 2, 1
			case "fast":
				config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 0, 30, 2, 1
			case "fast2":
				config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 20, 2, 1
			case "fast3":
				config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 10, 2, 1
			}
		}
		cipherKey := os.Getenv("GSNOVA_CIPHER_KEY")
		if len(cipherKey) > 0 {
			remote.ServerConf.Cipher.Key = cipherKey
			logger.Notice("Server cipher key overide by env:GSNOVA_CIPHER_KEY")
		}
		channel.SetDefaultMuxConfig(remote.ServerConf.Mux)
		channel.DefaultServerCipher = remote.ServerConf.Cipher
		remote.ServerConf.Cipher.AllowUsers(remote.ServerConf.Cipher.User)

		logger.InitLogger(remote.ServerConf.Log)

		logger.Info("Load server conf success.")
		confdata, _ := json.MarshalIndent(&remote.ServerConf, "", "    ")
		logger.Info("GSnova server:%s start with config:\n%s", channel.Version, string(confdata))
		remote.StartRemoteProxy()
	}

	if len(*pid) > 0 {
		ioutil.WriteFile(*pid, []byte(fmt.Sprintf("%d", os.Getpid())), os.ModePerm)
	}
	ch := make(chan int)
	<-ch
}

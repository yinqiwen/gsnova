package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

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
	otsListen := flag.String("ots", "", "Online trouble shooting listen address")
	pprofAddr := flag.String("pprof", "", "PProf trouble shooting listen address")
	version := flag.Bool("version", false, "Print version.")
	cmd := flag.Bool("cmd", false, "Launch gsnova by command line without config file.")
	isClient := flag.Bool("client", false, "Launch gsnova as client.")
	isServer := flag.Bool("server", false, "Launch gsnova as server.")
	pid := flag.String("pid", ".gsnova.pid", "PID file")
	conf := flag.String("conf", "", "Config file of gsnova.")
	key := flag.String("key", "809240d3a021449f6e67aa73221d42df942a308a", "Cipher key for transmission between local&remote.")
	log := flag.String("log", "color,gsnova.log", "Log file setting")
	window := flag.String("window", "", "Max mux stream window size, default 512K")
	windowRefresh := flag.String("window_refresh", "", "Mux stream window refresh size, default 32K")
	pingInterval := flag.Int("ping_interval", 30, "Channel ping interval seconds.")
	streamIdle := flag.Int("stream_idle", 10, "Mux stream idle timout seconds.")
	user := flag.String("user", "gsnova", "Username for remote server to authorize.")
	var whilteList, blackList channel.HopServers
	flag.Var(&whilteList, "whitelist", "Proxy whitelist item config")
	flag.Var(&blackList, "blackList", "Proxy blacklist item config")
	gcInterval := flag.Int("gc_interval", -1, "Manual GC every interval secs.")

	//client options
	admin := flag.String("admin", "", "Client Admin listen address")
	cnip := flag.String("cnip", "./cnipset.txt", "China IP list.")
	mitm := flag.Bool("mitm", false, "Launch gsnova as a MITM Proxy")
	httpDumpDest := flag.String("httpdump.dst", "", "HTTP Dump destination file or http url")
	var httpDumpFilters channel.HopServers
	flag.Var(&httpDumpFilters, "httpdump.filter", "HTTP Dump Domain Filter, eg:*.google.com")
	var hops, forwards channel.HopServers
	home, _ := filepath.Split(path)
	hosts := flag.String("hosts", "./hosts.json", "Hosts file of gsnova client.")
	flag.Var(&hops, "remote", "Next remote proxy hop server to connect for client, eg:wss://xxx.paas.com")
	flag.Var(&forwards, "forward", "Forward connection to specified address")
	p2p := flag.String("p2p", "", "P2P Token.")
	servable := flag.Bool("servable", false, "Client as a proxy server for peer p2p client")
	proxy := flag.String("proxy", "", "Proxy setting to connect remote server.")
	upnpPort := flag.Int("upnp", 0, "UPNP port to expose for p2p.")
	p2s2p := flag.Bool("p2s2p", false, "Connect two peers by P2S2P mode.")

	//client or server listen
	var listens channel.HopServers
	flag.Var(&listens, "listen", "Listen on address.")

	//server options
	tlsKey := flag.String("tls.key", "", "TLS Key file")
	tlsCert := flag.String("tls.cert", "", "TLS Cert file")

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

	if len(*otsListen) > 0 {
		err := ots.StartTroubleShootingServer(*otsListen)
		if nil != err {
			logger.Error("Failed to start admin server with reason:%v", err)
		}
	}
	if len(*pprofAddr) > 0 {
		go func() {
			http.ListenAndServe(*pprofAddr, nil)
		}()

	}
	if *gcInterval > 0 {
		go func() {
			for {
				runtime.GC()
				debug.FreeOSMemory()
				time.Sleep(time.Duration(*gcInterval) * time.Second)
			}
		}()
	}
	if runAsClient {
		options := local.ProxyOptions{
			Home:  home,
			Hosts: *hosts,
			CNIP:  *cnip,
		}
		local.GConf.UPNPExposePort = *upnpPort
		if *cmd {
			if len(hops) == 0 {
				logger.Error("At least one -hop argument required.", err)
				flag.PrintDefaults()
				return
			}
			if len(listens) == 0 {
				if len(*p2p) == 0 || !(*servable) {
					logger.Error("At least one -listen argument required.", err)
					flag.PrintDefaults()
					return
				}
			}
			local.GConf.Admin.Listen = *admin
			channelName := "default"
			if strings.EqualFold(hops[0], channel.DirectChannelName) {
				channelName = channel.DirectChannelName
			}
			local.GConf.Mux.MaxStreamWindow = *window
			local.GConf.Mux.StreamMinRefresh = *windowRefresh
			local.GConf.Mux.StreamIdleTimeout = *streamIdle

			local.GConf.Cipher.Key = *key
			local.GConf.Cipher.Method = "auto"
			local.GConf.Cipher.User = *user
			local.GConf.Log = strings.Split(*log, ",")
			for _, lis := range listens {
				proxyConf := local.ProxyConfig{}
				proxyConf.MITM = *mitm
				proxyConf.Local = lis
				proxyConf.HTTPDump.Dump = *httpDumpDest
				proxyConf.HTTPDump.Domain = httpDumpFilters
				proxyConf.PAC = []local.PACConfig{{Remote: channelName}}
				local.GConf.Proxy = append(local.GConf.Proxy, proxyConf)
			}
			for i, forward := range forwards {
				if len(local.GConf.Proxy) > i {
					local.GConf.Proxy[i].Forward = forward
				}
			}

			if !strings.EqualFold(hops[0], channel.DirectChannelName) {
				ch := channel.ProxyChannelConfig{}
				ch.Enable = true
				ch.Name = "default"
				ch.ConnsPerServer = 3
				ch.HeartBeatPeriod = *pingInterval
				ch.ServerList = []string{hops[0]}
				ch.Hops = hops[1:]
				ch.P2PToken = *p2p
				ch.P2S2PEnable = *p2s2p

				ch.Proxy = *proxy
				local.GConf.Channel = []channel.ProxyChannelConfig{ch}
			}

			local.GConf.ProxyLimit.WhiteList = whilteList
			local.GConf.ProxyLimit.BlackList = blackList
			if len(whilteList) == 0 && len(blackList) == 0 && *servable {
				local.GConf.ProxyLimit.WhiteList = []string{"*"}
			}
			remote.InitDefaultConf()
			remote.ServerConf.Cipher = local.GConf.Cipher
			remote.ServerConf.Mux = local.GConf.Mux
			remote.ServerConf.Cipher.AllowUsers(remote.ServerConf.Cipher.User)
			channel.DefaultServerCipher = remote.ServerConf.Cipher

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
			if len(listens) == 0 {
				logger.Error("At least one -listen argument required.", err)
				flag.PrintDefaults()
				return
			}
			for _, lis := range listens {
				var lisCfg remote.ServerListenConfig
				lisCfg.Listen = lis
				if len(*tlsCert) > 0 && len(*tlsKey) > 0 {
					lisCfg.Cert = *tlsCert
					lisCfg.Key = *tlsKey
				}
				lisCfg.KCParams.InitDefaultConf()
				config := &lisCfg.KCParams
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

				remote.ServerConf.Server = append(remote.ServerConf.Server, lisCfg)
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

		cipherKey := os.Getenv("GSNOVA_CIPHER_KEY")
		if len(cipherKey) > 0 {
			remote.ServerConf.Cipher.Key = cipherKey
			logger.Notice("Server cipher key overide by env:GSNOVA_CIPHER_KEY")
		}
		channel.DefaultServerRateLimit = remote.ServerConf.RateLimit
		channel.SetDefaultMuxConfig(remote.ServerConf.Mux)
		remote.ServerConf.Cipher.AllowUsers(remote.ServerConf.Cipher.User)
		channel.DefaultServerCipher = remote.ServerConf.Cipher

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

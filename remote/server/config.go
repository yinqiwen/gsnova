package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
)

type TLServerConfig struct {
	Cert   string
	Key    string
	Listen string
}

// Config for server
type KCPServerConfig struct {
	channel.KCPConfig
	Listen string
}

type QUICServerConfig struct {
	Listen string
	Cert   string
	Key    string
}

type HTTPServerConfig struct {
	Listen string
}
type HTTP2ServerConfig struct {
	Listen string
	Cert   string
	Key    string
}

type TCPServerConfig struct {
	Listen string
}

type ServerConfig struct {
	AdminListen string
	Cipher      channel.CipherConfig
	Mux         channel.MuxConfig
	Log         []string
	TLS         TLServerConfig
	KCP         KCPServerConfig
	QUIC        QUICServerConfig
	HTTP        HTTPServerConfig
	TCP         TCPServerConfig
	HTTP2       HTTP2ServerConfig
}

var ServerConf ServerConfig

func initDefaultConf() {
	ServerConf.KCP.Mode = "fast"
	ServerConf.KCP.MTU = 1350
	ServerConf.KCP.SndWnd = 1024
	ServerConf.KCP.RcvWnd = 1024
	ServerConf.KCP.DataShard = 10
	ServerConf.KCP.ParityShard = 3
	ServerConf.KCP.DSCP = 30
	ServerConf.KCP.AckNodelay = true
	ServerConf.KCP.NoDelay = 0
	ServerConf.KCP.Interval = 50
	ServerConf.KCP.Resend = 0
	ServerConf.KCP.Interval = 50
	ServerConf.KCP.NoCongestion = 0
	ServerConf.KCP.SockBuf = 4194304
}

func init() {
	cmd := flag.Bool("cmd", false, "Launch server by command line without config file.")
	key := flag.String("key", "", "Crypto key setting")
	logging := flag.String("log", "", "Server log setting, , split by ','")
	allow := flag.String("allow", "", "Allowed users, split by ','")
	conf := flag.String("conf", "server.json", "Server config file")
	httpServer := flag.String("http", "", "HTTP/Websocket listen address")
	http2Server := flag.String("http2", "", "HTTP2 listen address")
	tcpServer := flag.String("tcp", "", "TCP listen address")
	quicServer := flag.String("quic", "", "QUIC listen address")
	kcpServer := flag.String("kcp", "", "KCP listen address")
	tlsServer := flag.String("tls", "", "TLS listen address")
	admin := flag.String("admin", "", "Admin listen address")
	window := flag.String("window", "", "Max mux stream window size, default 256K")
	windowRefresh := flag.String("window_refresh", "", "Mux stream window refresh size, default 32K")
	pid := flag.String("pid", ".gsnova.pid", "PID file")

	flag.Parse()

	initDefaultConf()
	if !(*cmd) {
		if _, err := os.Stat(*conf); nil == err {
			logger.Info("Load server conf from file:%s", *conf)
			data, err := helper.ReadWithoutComment(*conf, "//")
			//data, err := ioutil.ReadFile(file)
			if nil == err {
				err = json.Unmarshal(data, &ServerConf)
			}
			if nil != err {
				logger.Error("Failed to load server config:%s for reason:%v", *conf, err)
				return
			}
		}
	}
	if len(*admin) > 0 {
		ServerConf.AdminListen = *admin
	}
	if len(*httpServer) > 0 {
		ServerConf.HTTP.Listen = *httpServer
	}
	if len(*tcpServer) > 0 {
		ServerConf.TCP.Listen = *tcpServer
	}
	if len(*quicServer) > 0 {
		ServerConf.QUIC.Listen = *quicServer
	}
	if len(*kcpServer) > 0 {
		ServerConf.KCP.Listen = *kcpServer
	}
	if len(*http2Server) > 0 {
		ServerConf.HTTP2.Listen = *http2Server
	}
	if len(*tlsServer) > 0 {
		ServerConf.TLS.Listen = *tlsServer
	}
	if len(*key) > 0 {
		ServerConf.Cipher.Key = *key
	}
	if len(*allow) > 0 {
		ServerConf.Cipher.User = *allow
		ServerConf.Cipher.AllowUsers(*allow)
		//ServerConf.AllowedUser = strings.Split(*allow, ",")
	} else {
		ServerConf.Cipher.AllowUsers(ServerConf.Cipher.User)
	}
	if len(*logging) > 0 {
		ServerConf.Log = strings.Split(*logging, ",")
	}
	if len(*window) > 0 {
		ServerConf.Mux.MaxStreamWindow = *window
	}
	if len(*windowRefresh) > 0 {
		ServerConf.Mux.StreamMinRefresh = *windowRefresh
	}

	if len(ServerConf.KCP.Listen) > 0 {
		config := &ServerConf.KCP
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
	channel.SetDefaultMuxConfig(ServerConf.Mux)
	channel.DefaultServerCipher = ServerConf.Cipher
	logger.InitLogger(ServerConf.Log)
	cipherKey := os.Getenv("GSNOVA_CIPHER_KEY")
	if len(cipherKey) > 0 {
		ServerConf.Cipher.Key = cipherKey
		logger.Notice("Server cipher key overide by env:GSNOVA_CIPHER_KEY")
	}

	logger.Info("Load server conf success.")
	confdata, _ := json.MarshalIndent(&ServerConf, "", "    ")
	logger.Info("GSnova server:%s start with config:\n%s", channel.RemoteVersion, string(confdata))

	if len(*pid) > 0 {
		ioutil.WriteFile(*pid, []byte(fmt.Sprintf("%d", os.Getpid())), os.ModePerm)
	}
}

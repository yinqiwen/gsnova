package remote

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/pmux"
)

type CipherConfig struct {
	//Method string
	Key string
}

type TLServerConfig struct {
	Cert   string
	Key    string
	Listen string
}

// Config for server
type KCPServerConfig struct {
	Listen       string
	Mode         string
	MTU          int
	SndWnd       int
	RcvWnd       int
	DataShard    int
	ParityShard  int
	DSCP         int
	AckNodelay   bool
	NoDelay      int
	Interval     int
	Resend       int
	NoCongestion int
	SockBuf      int
}

type QUICServerConfig struct {
	Listen string
}

type HTTPServerConfig struct {
	Listen string
}

type TCPServerConfig struct {
	Listen string
}

type ServerConfig struct {
	AdminListen          string
	DialTimeout          int
	MaxDynamicPort       int
	DynamicPortLifeCycle int
	CandidateDynamicPort []int
	AllowedUser          []string
	Cipher               CipherConfig
	Log                  []string
	TLS                  TLServerConfig
	KCP                  KCPServerConfig
	QUIC                 QUICServerConfig
	HTTP                 HTTPServerConfig
	TCP                  TCPServerConfig
}

func (conf *ServerConfig) VerifyUser(user string) bool {
	if len(conf.AllowedUser) == 0 {
		return true
	}
	for _, u := range conf.AllowedUser {
		if u == user || u == "*" {
			//log.Printf("Valid user:%s", user)
			return true
		}
	}
	log.Printf("[ERROR]Invalid user:%s", user)
	return false
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

func InitialPMuxConfig() *pmux.Config {
	cfg := pmux.DefaultConfig()
	cfg.CipherKey = []byte(ServerConf.Cipher.Key)
	cfg.CipherMethod = mux.DefaultMuxCipherMethod
	cfg.CipherInitialCounter = mux.DefaultMuxInitialCipherCounter
	return cfg
}

func init() {
	key := flag.String("key", "", "Crypto key setting")
	// listen := flag.String("listen", "", "Server listen address")
	logging := flag.String("log", "stdout", "Server log setting, , split by ','")
	allow := flag.String("allow", "*", "Allowed users, split by ','")
	// dps := flag.String("dps", "", "Candidate dynamic ports")
	// ndp := flag.Uint("ndp", 0, "Max dynamic ports")
	conf := flag.String("conf", "server.json", "Server config file")

	httpServer := flag.String("http", "", "HTTP/Websocket listen address")
	tcpServer := flag.String("tcp", "", "TCP listen address")
	quicServer := flag.String("quic", "", "QUIC listen address")
	kcpServer := flag.String("kcp", "", "KCP listen address")

	flag.Parse()

	initDefaultConf()
	if _, err := os.Stat(*conf); os.IsNotExist(err) {
		if len(*key) == 0 {
			flag.PrintDefaults()
			return
		}
		// dpstrs := strings.Split(*dps, ",")
		// for _, s := range dpstrs {
		// 	i, err := strconv.Atoi(s)
		// 	if nil == err && i > 1024 && i < 65535 {
		// 		ServerConf.CandidateDynamicPort = append(ServerConf.CandidateDynamicPort, i)
		// 	}
		// }
		ServerConf.Log = strings.Split(*logging, ",")
		ServerConf.AllowedUser = strings.Split(*allow, ",")
		ServerConf.TCP.Listen = *tcpServer
		ServerConf.QUIC.Listen = *quicServer
		ServerConf.KCP.Listen = *kcpServer
		ServerConf.HTTP.Listen = *httpServer
		port := os.Getenv("PORT")
		if port == "" {
			port = os.Getenv("OPENSHIFT_GO_PORT")
		}
		if port == "" {
			port = os.Getenv("VCAP_APP_PORT")
		}
		host := os.Getenv("OPENSHIFT_GO_IP")
		if len(port) > 0 {
			ServerConf.HTTP.Listen = host + ":" + port
		}
		ServerConf.Cipher.Key = *key
		//ServerConf.MaxDynamicPort = int(*ndp)
	} else {
		data, err := helper.ReadWithoutComment(*conf, "//")
		//data, err := ioutil.ReadFile(file)
		if nil == err {
			err = json.Unmarshal(data, &ServerConf)
		}
		if nil != err {
			log.Fatalf("Failed to load server config:%s for reason:%v", *conf, err)
			return
		}
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

	logger.InitLogger(ServerConf.Log)
	key := os.Getenv("GSNOVA_CIPHER_KEY")
	if len(key) > 0 {
		ServerConf.Cipher.Key = key
		log.Printf("Server cipher key overide by env:GSNOVA_CIPHER_KEY")
	} else {
		log.Printf("Server cipher key not overide by env:GSNOVA_CIPHER_KEY")
	}

	log.Printf("Load server conf success.")
	confdata, _ := json.MarshalIndent(&ServerConf, "", "    ")
	log.Printf("Start with config:\n%s", string(confdata))
}

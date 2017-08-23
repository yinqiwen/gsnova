package remote

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/pmux"
)

type EncryptConfig struct {
	Method string
	Key    string
}

type TLServerConfig struct {
	Cert   string
	Key    string
	Listen string
}

// Config for server
type KCPServerConfig struct {
	Listen       string `json:"listen"`
	Target       string `json:"target"`
	Key          string `json:"key"`
	Crypt        string `json:"crypt"`
	Mode         string `json:"mode"`
	MTU          int    `json:"mtu"`
	SndWnd       int    `json:"sndwnd"`
	RcvWnd       int    `json:"rcvwnd"`
	DataShard    int    `json:"datashard"`
	ParityShard  int    `json:"parityshard"`
	DSCP         int    `json:"dscp"`
	NoComp       bool   `json:"nocomp"`
	AckNodelay   bool   `json:"acknodelay"`
	NoDelay      int    `json:"nodelay"`
	Interval     int    `json:"interval"`
	Resend       int    `json:"resend"`
	NoCongestion int    `json:"nc"`
	SockBuf      int    `json:"sockbuf"`
	KeepAlive    int    `json:"keepalive"`
	Log          string `json:"log"`
	SnmpLog      string `json:"snmplog"`
	SnmpPeriod   int    `json:"snmpperiod"`
	Pprof        bool   `json:"pprof"`
}

type ServerConfig struct {
	Listen               string
	QUICListen           string
	AdminListen          string
	MaxDynamicPort       int
	DynamicPortLifeCycle int
	CandidateDynamicPort []int
	Auth                 []string
	Encrypt              EncryptConfig
	Log                  []string
	TLS                  TLServerConfig
	KCP                  KCPServerConfig
}

func (conf *ServerConfig) VerifyUser(user string) bool {
	if len(conf.Auth) == 0 {
		return true
	}
	for _, u := range conf.Auth {
		if u == user || u == "*" {
			//log.Printf("Valid user:%s", user)
			return true
		}
	}
	log.Printf("[ERROR]Invalid user:%s", user)
	return false
}

var ServerConf ServerConfig

func InitialPMuxConfig() *pmux.Config {
	cfg := pmux.DefaultConfig()
	cfg.CipherKey = []byte(ServerConf.Encrypt.Key)
	cfg.CipherMethod = mux.DefaultMuxCipherMethod
	cfg.CipherInitialCounter = mux.DefaultMuxInitialCipherCounter
	return cfg
}

func init() {
	key := flag.String("key", "", "Crypto key setting")
	listen := flag.String("listen", "", "Server listen address")
	logging := flag.String("log", "stdout", "Server log setting, , split by ','")
	auth := flag.String("auth", "*", "Auth user setting, split by ','")
	dps := flag.String("dps", "", "Candidate dynamic ports")
	ndp := flag.Uint("ndp", 0, "Max dynamic ports")
	conf := flag.String("conf", "server.json", "Server config file")
	flag.Parse()

	if _, err := os.Stat(*conf); os.IsNotExist(err) {
		if len(*key) == 0 || len(*listen) == 0 {
			flag.PrintDefaults()
			return
		}
		dpstrs := strings.Split(*dps, ",")
		for _, s := range dpstrs {
			i, err := strconv.Atoi(s)
			if nil == err && i > 1024 && i < 65535 {
				ServerConf.CandidateDynamicPort = append(ServerConf.CandidateDynamicPort, i)
			}
		}
		ServerConf.Log = strings.Split(*logging, ",")
		ServerConf.Auth = strings.Split(*auth, ",")
		ServerConf.Listen = *listen
		ServerConf.Encrypt.Key = *key
		ServerConf.MaxDynamicPort = int(*ndp)
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

	logger.InitLogger(ServerConf.Log)
	log.Printf("Load server conf success.")
	log.Printf("ServerConf:%v", &ServerConf)
}

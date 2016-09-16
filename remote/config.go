package remote

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
)

type EncryptConfig struct {
	Method string
	Key    string
}

type TLServerConfig struct {
	Cert string
	Key  string
}

type ServerConfig struct {
	Listen               string
	AdminListen          string
	MaxDynamicPort       int
	DynamicPortLifeCycle int
	CandidateDynamicPort []int
	Auth                 []string
	Encrypt              EncryptConfig
	Log                  []string
	TLS                  TLServerConfig
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
	event.SetDefaultSecretKey(ServerConf.Encrypt.Method, ServerConf.Encrypt.Key)
}

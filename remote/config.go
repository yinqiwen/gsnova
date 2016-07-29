package remote

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/logger"
)

type EncryptConfig struct {
	Method string
	Key    string
}

type ServerConfig struct {
	Listen  string
	Auth    []string
	Encrypt EncryptConfig
	Log     []string
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
	file := "server.json"
	data, err := ioutil.ReadFile(file)
	if nil == err {
		err = json.Unmarshal(data, &ServerConf)
	}
	if nil != err {
		fmt.Printf("Failed to load server config:%s for reason:%v", file, err)
		return
	}
	logger.InitLogger(ServerConf.Log)
	log.Printf("Load server conf success.")
	log.Printf("ServerConf:%v", &ServerConf)
	event.SetDefaultSecretKey(ServerConf.Encrypt.Method, ServerConf.Encrypt.Key)
}

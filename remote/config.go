package remote

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/yinqiwen/gsnova/common/event"
)

type ServerConfig struct {
	Auth   []string
	RC4Key string
}

func (conf *ServerConfig) VerifyUser(user string) bool {
	if len(conf.Auth) == 0 {
		return true
	}
	for _, u := range conf.Auth {
		if u == user || u == "*" {
			log.Printf("Valid user:%s", user)
			return true
		}
	}
	log.Printf("[ERROR]Invalid user:%s", user)
	return false
}

var ServerConf ServerConfig

func init() {
	file := "gsnova-server.json"
	data, err := ioutil.ReadFile(file)
	if nil == err {
		err = json.Unmarshal(data, &ServerConf)
	}
	if nil != err {
		fmt.Printf("Failed to load server config:%s for reason:%v", file, err)
		return
	}
	log.Printf("Load server conf success.")
	log.Printf("ServerConf:%v", &ServerConf)
	event.SetDefaultRC4Key(ServerConf.RC4Key)
}

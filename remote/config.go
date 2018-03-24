package remote

import (
	"github.com/yinqiwen/gsnova/common/channel"
)

type ServerListenConfig struct {
	Listen   string
	Cert     string
	Key      string
	KCParams channel.KCPConfig
}

type ServerConfig struct {
	Cipher channel.CipherConfig
	Mux    channel.MuxConfig
	Log    []string
	Server []ServerListenConfig
}

var ServerConf ServerConfig

func InitDefaultConf() {
	ServerConf.Mux.StreamIdleTimeout = 10
	ServerConf.Mux.SessionIdleTimeout = 300
	for _, lis := range ServerConf.Server {
		lis.KCParams.InitDefaultConf()
	}

}

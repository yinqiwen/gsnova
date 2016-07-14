package proxy

import (
	"github.com/yinqiwen/gsnova/common/logger"
)

var GConf LocalConfig

type ProxyConfig struct {
	Local  string
	Remote string
}

type PAASConfig struct {
	Enable         bool
	ServerList     []string
	ConnsPerServer int
}

type LocalConfig struct {
	Log    logger.LoggerConfig
	User   string
	Passwd string
	Proxy  []ProxyConfig
	PAAS   PAASConfig
}

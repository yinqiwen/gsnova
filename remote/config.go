package remote

import (
	"github.com/yinqiwen/gsnova/common/channel"
)

type TLServerConfig struct {
	Cert   string
	Key    string
	Listen string
}

// Config for server
type KCPServerConfig struct {
	Listen string
	Params channel.KCPConfig
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
	Cipher channel.CipherConfig
	Mux    channel.MuxConfig
	Log    []string
	TLS    TLServerConfig
	KCP    KCPServerConfig
	QUIC   QUICServerConfig
	HTTP   HTTPServerConfig
	TCP    TCPServerConfig
	HTTP2  HTTP2ServerConfig
}

var ServerConf ServerConfig

func InitDefaultConf() {
	ServerConf.Mux.IdleTimeout = 300
	ServerConf.KCP.Params.InitDefaultConf()
}

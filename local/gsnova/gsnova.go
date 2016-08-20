package gsnova

import (
	"github.com/getlantern/netx"
	_ "github.com/yinqiwen/gsnova/local/handler/direct"
	_ "github.com/yinqiwen/gsnova/local/handler/gae"
	_ "github.com/yinqiwen/gsnova/local/handler/paas"
	_ "github.com/yinqiwen/gsnova/local/handler/vps"
	"github.com/yinqiwen/gsnova/local/proxy"
)

func StartLocalProxy(dir string) error {
	return proxy.Start(dir)

}

func StopLocalProxy() error {
	netx.Reset()
	return proxy.Stop()
}

//SyncConfig sync config files from running gsnova instance
func SyncConfig(addr string, localDir string) error {
	return proxy.SyncConfig(addr, localDir)
}

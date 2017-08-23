package gsnova

import (
	"github.com/getlantern/netx"
	_ "github.com/yinqiwen/gsnova/local/channel/direct"
	_ "github.com/yinqiwen/gsnova/local/channel/http"
	_ "github.com/yinqiwen/gsnova/local/channel/quic"
	_ "github.com/yinqiwen/gsnova/local/channel/tcp"
	_ "github.com/yinqiwen/gsnova/local/channel/websocket"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type EventMonitor interface {
	OnEvent(code int, desc string) error
}

func StartLocalProxy(dir string, monitor EventMonitor) error {
	if nil != monitor {
		return proxy.Start(dir, monitor.OnEvent)
	} else {
		return proxy.Start(dir, nil)
	}

}

func StopLocalProxy() error {
	netx.Reset()
	return proxy.Stop()
}

//SyncConfig sync config files from running gsnova instance
func SyncConfig(addr string, localDir string) (bool, error) {
	return proxy.SyncConfig(addr, localDir)
}

package gsnova

import (
	"github.com/yinqiwen/gsnova/common/netx"
	_ "github.com/yinqiwen/gsnova/local/channel/direct"
	_ "github.com/yinqiwen/gsnova/local/channel/http"
	_ "github.com/yinqiwen/gsnova/local/channel/kcp"
	_ "github.com/yinqiwen/gsnova/local/channel/quic"
	_ "github.com/yinqiwen/gsnova/local/channel/ssh"
	_ "github.com/yinqiwen/gsnova/local/channel/tcp"
	_ "github.com/yinqiwen/gsnova/local/channel/websocket"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type EventMonitor interface {
	OnEvent(code int, desc string) error
}

type testProc struct {
}

func (t *testProc) Protect(fileDescriptor int) error {
	return nil
}

func StartLocalProxy(dir string, monitor EventMonitor) error {
	//ProtectConnections("114.114.114.114", &testProc{})
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

package gsnova

import (
	"github.com/yinqiwen/gsnova/common/netx"
	_ "github.com/yinqiwen/gsnova/local/channel/direct"
	_ "github.com/yinqiwen/gsnova/local/channel/http"
	_ "github.com/yinqiwen/gsnova/local/channel/http2"
	_ "github.com/yinqiwen/gsnova/local/channel/kcp"
	_ "github.com/yinqiwen/gsnova/local/channel/quic"
	_ "github.com/yinqiwen/gsnova/local/channel/ssh"
	_ "github.com/yinqiwen/gsnova/local/channel/tcp"
	_ "github.com/yinqiwen/gsnova/local/channel/websocket"
	"github.com/yinqiwen/gsnova/local/proxy"
)

// type testProc struct {
// }

// func (t *testProc) Protect(fileDescriptor int) error {
// 	return nil
// }

func StartLocalProxy(home string, conf string, hosts string, watchConf bool) error {
	options := proxy.ProxyOptions{
		Home:      home,
		Config:    conf,
		Hosts:     hosts,
		WatchConf: watchConf,
	}
	// ProtectConnections("114.114.114.114", &testProc{})
	return proxy.Start(options)
}

func StopLocalProxy() error {
	netx.Reset()
	return proxy.Stop()
}

//SyncConfig sync config files from running gsnova instance
func SyncConfig(addr string, localDir string) (bool, error) {
	return proxy.SyncConfig(addr, localDir)
}

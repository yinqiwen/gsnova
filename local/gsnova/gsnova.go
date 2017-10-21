package gsnova

import (
	_ "github.com/yinqiwen/gsnova/common/channel/common"
	"github.com/yinqiwen/gsnova/common/netx"
	"github.com/yinqiwen/gsnova/local/proxy"
)

func init() {

}

// type testProc struct {
// }

// func (t *testProc) Protect(fileDescriptor int) error {
// 	return nil
// }

func StartLocalProxy(home string, conf string, hosts string, cnip string, watchConf bool) error {
	options := proxy.ProxyOptions{
		Home:      home,
		Config:    conf,
		Hosts:     hosts,
		CNIP:      cnip,
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

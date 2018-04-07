package gsnova

import (
	_ "github.com/yinqiwen/gsnova/common/channel/common"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/netx"
	"github.com/yinqiwen/gsnova/local"
)

func init() {

}

// type testProc struct {
// }

// func (t *testProc) Protect(fileDescriptor int) error {
// 	return nil
// }

func StartLocalProxy(home string, conf string, hosts string, cnip string, watchConf bool) error {
	options := local.ProxyOptions{
		Home:      home,
		Config:    conf,
		Hosts:     hosts,
		CNIP:      cnip,
		WatchConf: watchConf,
	}
	// ProtectConnections("114.114.114.114", &testProc{})
	return local.Start(options)
}

func StopLocalProxy() error {
	netx.Reset()
	return local.Stop()
}

//SyncConfig sync config files from running gsnova instance
func SyncConfig(addr string, localDir string) (bool, error) {
	return local.SyncConfig(addr, localDir)
}

func CreateMITMCA(dir string) error {
	return helper.CreateRootCA(dir + "/MITM")
}

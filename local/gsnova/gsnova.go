package gsnova

import (
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
	return proxy.Stop()
}

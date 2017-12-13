// +build  android !linux

package local

import (
	"fmt"
	"net"

	"github.com/yinqiwen/gsnova/common/logger"
)

func getOrinalTCPRemoteAddr(conn net.Conn) (net.Conn, net.IP, uint16, error) {
	return nil, nil, 0, fmt.Errorf("'getOrinalTCPRemoteAddr' Not supported in current system")
}

func startTransparentUDProxy(addr string, proxy *ProxyConfig) {
	logger.Error("'startTransparentUDProxy' Not supported in current system")
}

func enableTransparentSocketMark(v int) {
	logger.Error("'enableTransparentSocketMark' Not supported in current system")
}

func supportTransparentProxy() bool {
	return false
}

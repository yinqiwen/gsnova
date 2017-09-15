// +build !linux

package proxy

import (
	"fmt"
	"net"

	"github.com/yinqiwen/gsnova/common/logger"
)

func getOrinalTCPRemoteAddr(conn net.Conn) (net.Conn, net.IP, uint16, error) {
	return nil, nil, 0, fmt.Errorf("Not supported for windows for 'getOrinalTCPRemoteAddr'")
}

func startTransparentUDProxy(addr string, proxy *ProxyConfig) {
	logger.Error("'startTransparentUDProxy' Not supported")
}

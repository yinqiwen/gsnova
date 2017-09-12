// +build windows

package proxy

import (
	"fmt"
	"net"
)

func getOrinalTCPRemoteAddr(conn net.Conn) (net.Conn, net.IP, uint16, error) {
	return nil, nil, 0, fmt.Errorf("Not supported for windows for 'getOrinalTCPRemoteAddr'")
}

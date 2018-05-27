// +build windows

package protector

import (
	"context"
	"net"
)

func ListenTCP(laddr *net.TCPAddr, options *NetOptions) (net.Listener, error) {
	return net.ListenTCP("tcp", laddr)
}

func DialContextOptions(ctx context.Context, network, addr string, opt *NetOptions) (net.Conn, error) {
	if nil != opt && opt.DialTimeout > 0 {
		return net.DialTimeout(network, addr, opt.DialTimeout)
	} else {
		return net.Dial(network, addr)
	}
}

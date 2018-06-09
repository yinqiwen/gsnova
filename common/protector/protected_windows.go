// +build windows

package protector

import (
	"context"
	"net"
)

func SupportReusePort() bool {
	return true
}

func ListenTCP(laddr *net.TCPAddr, options *NetOptions) (net.Listener, error) {
	return net.ListenTCP("tcp", laddr)
}

func DialContextOptions(ctx context.Context, network, addr string, opt *NetOptions) (net.Conn, error) {
	dialer := &net.Dialer{}
	if nil != opt {
		dialer.LocalAddr, _ = net.ResolveTCPAddr(network, opt.LocalAddr)
		dialer.Timeout = opt.DialTimeout
	}
	return dialer.Dial(network, addr)
}

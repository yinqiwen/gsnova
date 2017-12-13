// +build !windows

package gsnova

import (
	"github.com/yinqiwen/gsnova/common/netx"
	"github.com/yinqiwen/gsnova/common/protector"
)

// SocketProtector is an interface for classes that can protect Android sockets,
// meaning those sockets will not be passed through the VPN.
type SocketProtector interface {
	Protect(fileDescriptor int) error
}

// ProtectConnections allows connections made by Lantern to be protected from
// routing via a VPN. This is useful when running Lantern as a VPN on Android,
// because it keeps Lantern's own connections from being captured by the VPN and
// resulting in an infinite loop.
func ProtectConnections(dnsServer string, p SocketProtector) {
	protector.Configure(p.Protect, dnsServer)
	//p := New(protector.Protect, dnsServer)
	netx.OverrideDial(protector.DialContext)
	netx.OverrideResolve(protector.Resolve)
	netx.OverrideListenUDP(protector.ListenUDP)
	netx.OverrideDialUDP(protector.DialUDP)
}

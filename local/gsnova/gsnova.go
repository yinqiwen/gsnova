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

// SocketProtector is an interface for classes that can protect Android sockets,
// meaning those sockets will not be passed through the VPN.
type SocketProtector interface {
	Protect(fileDescriptor int) error
}

// ProtectConnections allows connections made by Lantern to be protected from
// routing via a VPN. This is useful when running Lantern as a VPN on Android,
// because it keeps Lantern's own connections from being captured by the VPN and
// resulting in an infinite loop.
func ProtectConnections(dnsServer string, protector SocketProtector) {
	// p := protected.New(protector.Protect, dnsServer)
	// netx.OverrideDial(p.Dial)
	// netx.OverrideResolve(p.Resolve)
}

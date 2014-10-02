package godns

import (
	"net"
	"time"
)

var GoogleDNSServers = []string{"8.8.8.8", "8.8.4.4"}
var OpenDNSServers = []string{"208.67.222.222", "208.67.220.220"}

type dnsConfig struct {
	servers     []string // servers to use
	search      []string // suffixes to append to local name
	ndots       int      // number of dots in name to trigger absolute lookup
	timeout     int      // seconds before giving up on packet
	attempts    int      // lost packets before giving up on server
	rotate      bool     // round robin among servers
	net         string
	dialTimeout func(net, addr string, timeout time.Duration) (net.Conn, error)
}

func dnsConfigWithOptions(options *LookupOptions) (*dnsConfig, error) {
	conf := new(dnsConfig)
	conf.net = options.Net
	conf.servers = options.DNSServers
	conf.dialTimeout = options.DialTimeout
	//conf.servers = make([]string, 3)[0:0] // small, but the standard limit
	conf.search = make([]string, 0)
	conf.ndots = 1
	conf.timeout = 5
	conf.attempts = 2
	conf.rotate = false
	return conf, nil
}

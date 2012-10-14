package proxy

import (
	"net"
	"net/http"
	"strings"
)

var func_table = make(map[string]func(*http.Request) bool)

func init_spac_func() {
	func_table["IsHostInCN"] = isHostInCN
	func_table["IsBlockedByGFW"] = isBlockedByGFW
}

func invokeFilter(name string, req *http.Request) bool {
	if filter, exist := func_table[name]; exist {
		return filter(name)
	}
	return false
}

func isHostInCN(req *http.Request) bool {
	host := req.Host
	if strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}
	return false
}

func isBlockedByGFW(req *http.Request) bool {
	return false
}

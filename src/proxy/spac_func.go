package proxy

import (
	"misc/gfwlist"
	"misc/iprange"
	"net"
	"net/http"
	"strings"
	//"common"
//	"log"
)

var gfwList *gfwlist.GFWList
var ipfunc *iprange.IPRangeHolder
var func_table = make(map[string]func(*http.Request) bool)

func init_spac_func() {
	func_table["IsHostInCN"] = isHostInCN
	func_table["IsBlockedByGFW"] = isBlockedByGFW
}

func init_gfwlist_func(rule string) {
	gfwList, _ = gfwlist.Parse(rule)
}

func init_iprange_func(file string) {
	ipfunc, _ = iprange.Parse(file, "worldip.en.txt")
}

func invokeFilter(name string, req *http.Request) bool {
	not := false
	if strings.HasPrefix(name, "!") {
		name = name[1:]
		not = true
	}
	if filter, exist := func_table[name]; exist {
		if not {
			return !filter(req)
		}
		return filter(req)
	}
	return false
}

func isHostInCN(req *http.Request) bool {
	if nil == ipfunc {
		return true
	}
	host := req.Host
	if strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}
	//consider use trusted DNS
	ip, ok := trustedDNSLookup(host)
	if !ok {
		return false
	}
	ret := strings.EqualFold(ipfunc.FindCountry(ip), "CN")
	//	if ret {
	//	   log.Printf("Find country for host:%s is CN\n", host)
	//	}
	return ret
}

func isBlockedByGFW(req *http.Request) bool {
	if gfwList == nil {
		return true
	}
	return gfwList.IsBlockedByGFW(req)
}

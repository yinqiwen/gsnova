package proxy

import (
	"log"
	"misc/gfwlist"
	"misc/iprange"
	"net"
	"net/http"
	"strings"
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
	ipfunc, _ = iprange.ParseApnic(file)
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
	var ip string
	var ok bool
	if strings.HasSuffix(host, ".cn") {
		return true
	} else {
		if ips, err := net.LookupHost(host); nil == err && len(ips) > 0 {
			ip = ips[0]
			ok = true
		}
	}
	if !ok || nil == ipfunc {
		return false
	}
	country, err := ipfunc.FindCountry(ip)
	if nil != err {
		log.Printf("[WARN]Find country error:%v\n", err)
		return false
	}
	ret := strings.EqualFold(country, "CN")

	return ret
}

func isBlockedByGFW(req *http.Request) bool {
	if gfwList == nil {
		return true
	}
	return gfwList.IsBlockedByGFW(req)
}

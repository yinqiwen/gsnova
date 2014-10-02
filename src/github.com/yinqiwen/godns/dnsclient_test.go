package godns

import (
	//"net"
	"testing"
)

func TestLoopkupIPOverTcp(t *testing.T) {
	options := &LookupOptions{
		DNSServers: GoogleDNSServers, Net: "tcp"}
	addrs, err := LookupIP("www.google.com.hk", options)
	if nil != err {
		t.Error("Failed to load ini file for reason:" + err.Error())
		return
	}
	for _, ip := range addrs {
		t.Logf("%v\n", ip.String())
	}
	t.Fail()
}

func TestLoopkupIPOverUdp(t *testing.T) {
	options := &LookupOptions{
		DNSServers: GoogleDNSServers, Net: "udp"}
	addrs, err := LookupIP("www.google.com.hk", options)
	if nil != err {
		t.Error("Failed to load ini file for reason:" + err.Error())
		return
	}
	for _, ip := range addrs {
		t.Logf("%v\n", ip.String())
	}
	t.Fail()
}

package proxy

import (
	"github.com/yinqiwen/godns"
	"net"
	"sync"
	"time"
)

var blockVerifyCache = make(map[string]bool)
var blockVerifyMutex sync.Mutex
var blockVerifyTimeout = 5

var crlfDomainCache = make(map[string]bool)
var crlfDomainCacheMutex sync.Mutex

func getBlockVerifyCache(hostport string) (bool, bool) {
	blockVerifyMutex.Lock()
	defer blockVerifyMutex.Unlock()
	if v, exist := blockVerifyCache[hostport]; exist {
		return v, true
	}
	return false, false
}

func setBlockVerifyCache(hostport string, v bool) {
	blockVerifyMutex.Lock()
	blockVerifyCache[hostport] = v
	blockVerifyMutex.Unlock()
}

func expireBlockVerifyCache(hostport string) {
	blockVerifyMutex.Lock()
	delete(blockVerifyCache, hostport)
	blockVerifyMutex.Unlock()
}

func setDomainCRLFAttr(hostport string) {
	crlfDomainCacheMutex.Lock()
	crlfDomainCache[hostport] = true
	crlfDomainCacheMutex.Unlock()
}

func getDomainCRLFAttr(hostport string) bool {
	crlfDomainCacheMutex.Lock()
	defer crlfDomainCacheMutex.Unlock()
	_, exist := crlfDomainCache[hostport]
	return exist
}

func isTCPAddressBlocked(ip, port string) bool {
	addr := net.JoinHostPort(ip, port)
	if v, exist := getBlockVerifyCache(addr); exist {
		return v
	}
	c, err := net.DialTimeout("tcp", addr, time.Duration(blockVerifyTimeout)*time.Second)
	if nil != err {
		setBlockVerifyCache(addr, true)
		return true
	}
	c.Close()
	setBlockVerifyCache(addr, false)
	return false
}

func trustedDNSLookup(host string) (string, bool) {
	net := "tcp"
	//for DNSEncrypt
	if len(trustedDNS) > 0 && trustedDNS[0] == "127.0.0.1" {
		net = "udp"
	}
	options := &godns.LookupOptions{
		DNSServers: trustedDNS,
		Net:        net,
		CacheTTL:   godns.DNS_CACHE_TTL_SELF,
		OnlyIPv4:   true}
	if ips, err := godns.LookupIP(host, options); nil == err {
		for _, ip := range ips {
			if nil != ip.To4() {
				return ip.String(), true
			}
		}
	}
	return host, false
}

func trustedDNSQuery(host string, port string) (string, bool) {
	net := "tcp"
	//for DNSEncrypt
	if len(trustedDNS) > 0 && trustedDNS[0] == "127.0.0.1" {
		net = "udp"
	}
	options := &godns.LookupOptions{
		DNSServers: trustedDNS,
		Net:        net,
		CacheTTL:   godns.DNS_CACHE_TTL_SELF,
		OnlyIPv4:   true}
	if ips, err := godns.LookupIP(host, options); nil == err {
		for _, ip := range ips {
			if nil != ip.To4() {
				blocked := isTCPAddressBlocked(ip.String(), port)
				if !blocked {
					return ip.String(), true
				}
			}
		}
		return "", false
	}
	return "", false
}

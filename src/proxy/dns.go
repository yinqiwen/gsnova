package proxy

import (
	"github.com/yinqiwen/godns"
	//	"common"
	//	"encoding/json"
	//	"io"
	//	"os"
	"net"
	"sync"
	"time"
)

var dnsCache = make(map[string]DNSResult)
var dnsCacheMutex sync.Mutex
var blockVerifyTimeout = 5

type DNSResult struct {
	IP         map[string]bool
	Date       time.Time
	InjectCRLF bool
}

func getDNSCacheIP(hostport string) (string, DNSResult, bool) {
	dnsCacheMutex.Lock()
	defer dnsCacheMutex.Unlock()
	if v, exist := dnsCache[hostport]; exist {
		for k, reachable := range v.IP {
			if reachable {
				return k, v, true
			}
		}
		return "", v, true
	}
	return "", DNSResult{}, false
}

func setDNSCache(hostport string, v DNSResult) {
	dnsCacheMutex.Lock()
	dnsCache[hostport] = v
	dnsCacheMutex.Unlock()
}

func setDNSCacheCRLFAttr(hostport string) {
	dnsCacheMutex.Lock()
	if v, exist := dnsCache[hostport]; exist {
		v.InjectCRLF = true
		dnsCache[hostport] = v
	}
	dnsCacheMutex.Unlock()
}

func setDNSCacheBlocked(domainport, hostport string) {
	dnsCacheMutex.Lock()
	if v, exist := dnsCache[domainport]; exist {
		ip, _, _ := net.SplitHostPort(hostport)
		if _, found := v.IP[ip]; found {
			v.IP[ip] = false
			dnsCache[domainport] = v
		}
	}
	dnsCacheMutex.Unlock()
}

func isTCPAddressBlocked(domain, ip, port string) bool {
	addr := net.JoinHostPort(ip, port)
	//start := time.Now().UnixNano()
	c, err := net.DialTimeout("tcp", addr, time.Duration(blockVerifyTimeout)*time.Second)
	if nil != err {
		return true
	}
	//end := time.Now().UnixNano()
	c.Close()
	return false
}

func trustedDNSQuery(host string, port string) (string, bool) {
	hostport := net.JoinHostPort(host, port)
	if result, _, exist := getDNSCacheIP(hostport); exist {
		return result, len(result) > 0
	}
	net := "tcp"
	//for DNSEncrypt
	if len(trustedDNS) > 0 && trustedDNS[0] == "127.0.0.1" {
		net = "udp"
	}
	options := &godns.LookupOptions{
		DNSServers: trustedDNS,
		Net:        net,
		Cache:      true,
		OnlyIPv4:   true}
	cache := make(map[string]bool)
	result := ""
	if ips, err := godns.LookupIP(host, options); nil == err {
		for _, ip := range ips {
			if nil != ip.To4() {
				blocked := isTCPAddressBlocked(host, ip.String(), port)
				cache[ip.String()] = !blocked
				if !blocked && len(result) == 0 {
					result = ip.String()
				}
			}
		}
		setDNSCache(hostport, DNSResult{cache, time.Now(), false})
		return result, len(result) > 0
	}
	setDNSCache(hostport, DNSResult{cache, time.Now(), false})
	return "", false
}

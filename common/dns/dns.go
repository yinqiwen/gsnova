package dns

import (
	"context"
	"math/rand"
	"net"

	"github.com/miekg/dns"
	"github.com/yinqiwen/fdns"
	"github.com/yinqiwen/gotoolkit/cip"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/netx"
)

var LocalDNS *fdns.TrustedDNS

func pickIP(rr []dns.RR) string {
	for _, answer := range rr {
		if a, ok := answer.(*dns.A); ok {
			return a.A.String()
		}
	}
	for _, answer := range rr {
		if aaaa, ok := answer.(*dns.AAAA); ok {
			return aaaa.AAAA.String()
		}
	}
	return ""
}

func selectDNSServer(ss []string) string {
	var server string
	slen := len(ss)
	if slen == 1 {
		server = ss[0]
	} else {
		server = ss[rand.Intn(slen)]
	}
	return server
}

func getIPByDefaultResolver(domain string) (string, error) {
	addrs, err := net.DefaultResolver.LookupHost(context.Background(), domain)
	if nil == err && len(addrs) > 0 {
		return addrs[0], nil
	}
	return "", err
}

func DnsGetDoaminIP(domain string) (string, error) {
	if nil != LocalDNS {
		ips, err := LocalDNS.LookupA(domain)
		if len(ips) > 0 {
			return pickIP(ips), err
		}
	}
	return getIPByDefaultResolver(domain)
}

var CNIPSet *cip.CountryIPSet

type LocalDNSConfig struct {
	Listen     string
	TrustedDNS []string
	FastDNS    []string
	CNIPSet    string
}

func Init(conf *LocalDNSConfig) {
	cnipset, err := cip.LoadIPSet(conf.CNIPSet, "CN")
	if nil != err {
		logger.Error("Failed to load IP range file:%s with reason:%v", conf.CNIPSet, err)
	} else {
		CNIPSet = cnipset
	}
	cfg := &fdns.Config{}
	cfg.Listen = conf.Listen
	for _, s := range conf.FastDNS {
		ss := fdns.ServerConfig{
			Server:      s,
			Timeout:     500,
			MaxResponse: 1,
		}
		cfg.FastDNS = append(cfg.FastDNS, ss)
	}
	for _, s := range conf.TrustedDNS {
		ss := fdns.ServerConfig{
			Server:      s,
			Timeout:     1000,
			MaxResponse: 5,
		}
		cfg.TrustedDNS = append(cfg.TrustedDNS, ss)
	}
	cfg.MinTTL = 24 * 3600
	cfg.DialTimeout = netx.DialTimeout
	cfg.IsCNIP = func(ip net.IP) bool {
		if nil == CNIPSet {
			return false
		}
		return CNIPSet.IsInCountry(ip, "CN")
	}
	cfg.IsDomainPoisioned = func(domain string) int {
		//conf.GFWList.Load()
		return -1
	}
	LocalDNS, _ = fdns.NewTrustedDNS(cfg)
	if len(conf.Listen) > 0 {
		go func() {
			err := LocalDNS.Start()
			if nil != err {
				logger.Error("Failed to start dns server:%v", err)
			}
		}()
	}
}

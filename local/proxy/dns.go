package proxy

import (
	"math/rand"
	"net"

	"github.com/yinqiwen/gotoolkit/cip"

	"github.com/miekg/dns"
	"github.com/yinqiwen/fdns"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/netx"
)

var localDNS *fdns.TrustedDNS

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

func DnsGetDoaminIP(domain string) (string, error) {
	ips, err := localDNS.LookupA(domain)
	if len(ips) > 0 {
		return pickIP(ips), err
	}
	return "", err
}

var cnipConf string
var chinaIPSet *cip.CountryIPSet

func initDNS() {
	cnipset, err := cip.LoadIPSet(cnipConf, "CN")
	if nil != err {
		logger.Error("Failed to load IP range file:%s with reason:%v", cnipConf, err)
	} else {
		chinaIPSet = cnipset
	}
	cfg := &fdns.Config{}
	cfg.Listen = GConf.LocalDNS.Listen
	for _, s := range GConf.LocalDNS.FastDNS {
		ss := fdns.ServerConfig{
			Server:      s,
			Timeout:     500,
			MaxResponse: 1,
		}
		cfg.FastDNS = append(cfg.FastDNS, ss)
	}
	for _, s := range GConf.LocalDNS.TrustedDNS {
		ss := fdns.ServerConfig{
			Server:      s,
			Timeout:     800,
			MaxResponse: 5,
		}
		cfg.TrustedDNS = append(cfg.TrustedDNS, ss)
	}
	cfg.MinTTL = 24 * 3600
	cfg.DialTimeout = netx.DialTimeout
	cfg.IsCNIP = func(ip net.IP) bool {
		if nil == chinaIPSet {
			return false
		}
		return chinaIPSet.IsInCountry(ip, "CN")
	}
	localDNS, _ = fdns.NewTrustedDNS(cfg)
	if len(GConf.LocalDNS.Listen) > 0 {
		go func() {
			err := localDNS.Start()
			if nil != err {
				logger.Error("Failed to start dns server:%v", err)
			}
		}()
	}
}

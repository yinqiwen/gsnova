package proxy

import (
	"errors"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/getlantern/netx"
	"github.com/hashicorp/golang-lru"
	"github.com/miekg/dns"
	"github.com/yinqiwen/gsnova/local/protector"
)

var errNoDNServer = errors.New("No DNS server configured.")
var dnsCache *lru.Cache

func selectDNSServer(servers []string) string {
	serverLen := len(servers)
	server := servers[rand.Intn(serverLen)]
	if !strings.Contains(server, ":") {
		server = net.JoinHostPort(server, "53")
	}
	return server
}

type getConnIntf interface {
	GetConn() net.Conn
}

func dnsQuery(r *dns.Msg) (*dns.Msg, error) {
	network := "udp"
	if GConf.LocalDNS.TCPConnect {
		network = "tcp"
	}
	server := selectDNSServer(GConf.LocalDNS.TrustedDNS)
	log.Printf("DNS query to %s", server)
	c, err := netx.DialTimeout(network, server, 3*time.Second)
	if nil != err {
		return nil, err
	}
	defer c.Close()
	dnsConn := new(dns.Conn)
	if pc, ok := c.(getConnIntf); ok {
		c = pc.GetConn()
	}
	dnsConn.Conn = c
	dnsConn.WriteMsg(r)
	return dnsConn.ReadMsg()
}

func dnsQueryRaw(r []byte) ([]byte, error) {
	req := new(dns.Msg)
	req.Unpack(r)
	res, err := dnsQuery(req)
	if nil != err {
		return nil, err
	}
	return res.Pack()
}

func dnsGetDoaminIP(domain string) (string, error) {
	var record *protector.DNSRecord
	if nil != dnsCache {
		item, exist := dnsCache.Get(domain)
		if exist {
			record = item.(*protector.DNSRecord)
			if time.Now().After(record.ExpireAt) {
				record = nil
				dnsCache.Remove(domain)
			}
		}
	}

	if nil == record {
		var dnsServers []string
		if nil != mygfwlist {
			connReq, _ := http.NewRequest("CONNECT", "https://"+domain, nil)
			if !mygfwlist.IsBlockedByGFW(connReq) {
				dnsServers = GConf.LocalDNS.FastDNS
			}
		}
		if len(dnsServers) == 0 {
			dnsServers = GConf.LocalDNS.TrustedDNS
		}
		if len(dnsServers) == 0 {
			log.Printf("At least one DNS server need to be configured in 'FastDNS/TrustedDNS'")
			return "", errNoDNServer
		}
		server := selectDNSServer(dnsServers)
		log.Printf("DNS query %s to %s", domain, server)
		c, err := netx.DialTimeout("udp", server, 2*time.Second)
		if nil != err {
			return "", err
		}
		defer c.Close()
		c.SetReadDeadline(time.Now().Add(3 * time.Second))
		res, err := protector.DnsLookup(domain, c)
		if nil != err {
			return "", err
		}
		record, err = res.PickRecord()
		if nil != err {
			return "", err
		}
		if nil != dnsCache {
			dnsCache.Add(domain, record)
		}
	}
	return record.IP.String(), nil
}

func proxyDNS(w dns.ResponseWriter, r *dns.Msg) {
	dnsres, err := dnsQuery(r)
	if nil != err {
		log.Printf("DNS query error:%v", err)
		return
	}
	if nil != dnsres {
		w.WriteMsg(dnsres)
	}
}

func initDNS() {
	if GConf.LocalDNS.CacheSize > 0 {
		dnsCache, _ = lru.New(GConf.LocalDNS.CacheSize)
	}
	if len(GConf.LocalDNS.Listen) > 0 {
		err := dns.ListenAndServe(GConf.LocalDNS.Listen, "udp", dns.HandlerFunc(proxyDNS))
		if nil != err {
			log.Printf("Failed to start dns server:%v", err)
		}
	}
}

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
)

var errNoDNServer = errors.New("No DNS server configured.")
var dnsCache *lru.Cache

type dnsCacheRecord struct {
	expireAt time.Time
	res      *dns.Msg
}

func pickIP(res *dns.Msg) string {
	for _, answer := range res.Answer {
		if a, ok := answer.(*dns.A); ok {
			return a.A.String()
		}
	}
	return ""
}

func newDNSCacheRecord(res *dns.Msg) *dnsCacheRecord {
	record := new(dnsCacheRecord)
	record.res = res
	now := time.Now()
	for _, answer := range res.Answer {
		if a, ok := answer.(*dns.A); ok {
			record.expireAt = now.Add(time.Duration(a.Hdr.Ttl+10) * time.Second)
			break
		}
	}
	return record
}

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
	dnsServers := GConf.LocalDNS.TrustedDNS
	var record *dnsCacheRecord
	var domain string
	useTrustedDNS := true
	if len(r.Question) == 1 && r.Question[0].Qtype == dns.TypeA && dns.IsFqdn(r.Question[0].Name) {
		domain = r.Question[0].Name
		domain = domain[0 : len(domain)-1]
		if nil != dnsCache {
			item, exist := dnsCache.Get(domain)
			if exist {
				record = item.(*dnsCacheRecord)
				if time.Now().After(record.expireAt) {
					record = nil
					dnsCache.Remove(domain)
				} else {
					return record.res, nil
				}
			}
		}
		if nil != mygfwlist {
			connReq, _ := http.NewRequest("CONNECT", "https://"+domain, nil)
			isBlocked, _ := mygfwlist.FastMatchDoamin(connReq)
			if !isBlocked {
				dnsServers = GConf.LocalDNS.FastDNS
				useTrustedDNS = false
			}
		}
	}
	if len(dnsServers) == 0 {
		dnsServers = GConf.LocalDNS.TrustedDNS
		useTrustedDNS = true
	}
	if len(dnsServers) == 0 {
		log.Printf("At least one DNS server need to be configured in 'FastDNS/TrustedDNS'")
		return nil, errNoDNServer
	}
	server := selectDNSServer(dnsServers)
	network := "udp"
	if GConf.LocalDNS.TCPConnect && useTrustedDNS {
		network = "tcp"
	}
	log.Printf("DNS query to %s", server)
	c, err := netx.DialTimeout(network, server, 2*time.Second)
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
	dnsConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	res, err1 := dnsConn.ReadMsg()
	if nil == err1 && nil != dnsCache && len(domain) > 0 {
		record = newDNSCacheRecord(res)
		dnsCache.Add(domain, record)
	}
	return res, err1
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

func DnsGetDoaminIP(domain string) (string, error) {
	m := new(dns.Msg)
	m.Id = dns.Id()
	m.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	m.RecursionDesired = true
	res, err := dnsQuery(m)
	if nil != err {
		return "", err
	}
	return pickIP(res), nil
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

package proxy

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru"
	"github.com/miekg/dns"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/netx"
)

var errNoDNServer = errors.New("No DNS server configured.")
var errDNSQuryFail = errors.New("DNS query failed.")
var dnsCache *lru.Cache

type dnsCacheRecord struct {
	expireAt time.Time
	ipv4Res  *dns.Msg
	ipv6Res  *dns.Msg
}

func pickIP(res *dns.Msg) string {
	for _, answer := range res.Answer {
		if a, ok := answer.(*dns.A); ok {
			return a.A.String()
		}
	}
	for _, answer := range res.Answer {
		if aaaa, ok := answer.(*dns.AAAA); ok {
			return aaaa.AAAA.String()
		}
	}
	return ""
}

func newDNSCacheRecord(record *dnsCacheRecord, res *dns.Msg) *dnsCacheRecord {
	if nil == record {
		record = new(dnsCacheRecord)
	}
	ipv4 := false
	now := time.Now()
	for _, answer := range res.Answer {
		var hdr dns.RR_Header
		if a, ok := answer.(*dns.A); ok {
			hdr = a.Hdr
			ipv4 = true
		} else if aaaa, ok := answer.(*dns.AAAA); ok {
			hdr = aaaa.Hdr
			ipv4 = false
		}
		record.expireAt = now.Add(time.Duration(hdr.Ttl+10) * time.Second)
		break
	}
	if ipv4 {
		record.ipv4Res = res
	} else {
		record.ipv6Res = res
	}
	return record
}

func selectDNSServer(servers []string) string {
	var server string
	serverLen := len(servers)
	if serverLen == 1 {
		server = servers[0]
	} else {
		server = servers[rand.Intn(serverLen)]
	}
	if !strings.Contains(server, ":") {
		server = net.JoinHostPort(server, "53")
	}
	return server
}

type getConnIntf interface {
	GetConn() net.Conn
}

func dnsQuery(r *dns.Msg, viaGW bool) (*dns.Msg, error) {
	dnsServers := GConf.LocalDNS.TrustedDNS
	var record *dnsCacheRecord
	var domain string
	useTrustedDNS := true

	if len(r.Question) == 1 && dns.IsFqdn(r.Question[0].Name) {
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
					if r.Question[0].Qtype == dns.TypeA && nil != record.ipv4Res {
						return record.ipv4Res, nil
					} else if r.Question[0].Qtype == dns.TypeAAAA && nil != record.ipv6Res {
						return record.ipv6Res, nil
					}
				}
			}
		}
		if viaGW {
			matchResult := GConf.UDPGW.matchDNS(domain)
			if len(matchResult) > 0 {
				return dnsGenResponse(r, matchResult), nil
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
	} else {
		logger.Debug("###DNS with %v", r.Question)
	}
	if len(dnsServers) == 0 {
		dnsServers = GConf.LocalDNS.TrustedDNS
		useTrustedDNS = true
	}
	if len(dnsServers) == 0 {
		logger.Debug("At least one DNS server need to be configured in 'FastDNS/TrustedDNS'")
		return nil, errNoDNServer
	}
	server := selectDNSServer(dnsServers)
	network := "udp"
	if GConf.LocalDNS.TCPConnect && useTrustedDNS {
		network = "tcp"
	}
	logger.Debug("DNS query %s to %s", domain, server)
	for retry := 0; retry < 3; retry++ {
		c, err := netx.DialTimeout(network, server, 1*time.Second)
		if nil != err {
			return nil, err
		}
		dnsConn := new(dns.Conn)
		if pc, ok := c.(getConnIntf); ok {
			c = pc.GetConn()
		}
		dnsConn.Conn = c
		dnsConn.WriteMsg(r)
		dnsConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		res, err1 := dnsConn.ReadMsg()
		if nil == err1 && nil != dnsCache && len(domain) > 0 {
			record = newDNSCacheRecord(record, res)
			dnsCache.Add(domain, record)
		}
		c.Close()
		if nil == err1 {
			return res, nil
		}
	}
	return nil, errDNSQuryFail
}

func dnsQueryRaw(r []byte, viaGW bool) ([]byte, error) {
	req := new(dns.Msg)
	req.Unpack(r)
	res, err := dnsQuery(req, viaGW)
	if nil != err {
		return nil, err
	}
	return res.Pack()
}

func dnsGenResponse(req *dns.Msg, ip string) *dns.Msg {
	res := &dns.Msg{}
	res.SetReply(req)
	res.Compress = false
	switch req.Opcode {
	case dns.OpcodeQuery:
		for _, q := range res.Question {
			switch q.Qtype {
			case dns.TypeA:
				rr, _ := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, ip))
				res.Answer = append(res.Answer, rr)
			}
		}
	}
	return res
}

func dnsGenResponsePacket(raw []byte, ip string) ([]byte, error) {
	req := new(dns.Msg)
	req.Unpack(raw)
	res := dnsGenResponse(req, ip)
	return res.Pack()
}

func DnsGetDoaminIP(domain string) (string, error) {
	m := new(dns.Msg)
	m.Id = dns.Id()
	m.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	m.RecursionDesired = true
	res, err := dnsQuery(m, false)
	if nil != err {
		if err == errNoDNServer {
			addrs, err := net.DefaultResolver.LookupHost(context.Background(), domain)
			if nil == err {
				return addrs[0], nil
			}
			return "", nil
		}
		return "", err
	}
	return pickIP(res), nil
}

func proxyDNS(w dns.ResponseWriter, r *dns.Msg) {
	dnsres, err := dnsQuery(r, false)
	if nil != err {
		logger.Error("DNS query error:%v", err)
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
			logger.Error("Failed to start dns server:%v", err)
		}
	}
}

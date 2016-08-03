package proxy

import (
	"log"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/getlantern/netx"
	"github.com/miekg/dns"
)

func dnsQuery(r *dns.Msg) (*dns.Msg, error) {
	serverLen := len(GConf.LocalDNS.TrustedDNS)
	network := "udp"
	if GConf.LocalDNS.TCPConnect {
		network = "tcp"
	}
	server := GConf.LocalDNS.TrustedDNS[rand.Intn(serverLen)]
	if !strings.Contains(server, ":") {
		server = net.JoinHostPort(server, "53")
	}
	log.Printf("DNS query to %s", server)
	c, err := netx.DialTimeout(network, server, 3*time.Second)
	if nil != err {
		return nil, err
	}
	defer c.Close()
	dnsConn := new(dns.Conn)
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

func startLocalDNS(addr string) error {
	err := dns.ListenAndServe(addr, "udp", dns.HandlerFunc(proxyDNS))
	if nil != err {
		log.Printf("Failed to start dns server:%v", err)
	}
	return err
}

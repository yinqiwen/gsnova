package proxy

// import (
// 	"log"
// 	"math/rand"
// 	"time"

// 	"github.com/miekg/dns"
// )

// func proxyDNS(w dns.ResponseWriter, r *dns.Msg) {
// 	server := GConf.LocalDNS.DNS[rand.Intn(len(GConf.LocalDNS.DNS))] + ":53"
// 	c, err := dns.Dial("udp", server)
// 	if nil != err {
// 		log.Printf("dns dial error %v", err)
// 		return
// 	}
// 	defer c.Close()
// 	start := time.Now()
// 	c.WriteMsg(r)

// 	var dnsres *dns.Msg
// 	count := 0
// 	for {
// 		c.SetReadDeadline(start.Add(500 * time.Millisecond))
// 		res, err := c.ReadMsg()
// 		if nil != err {
// 			log.Printf("read1 dial error %v", err)
// 			break
// 		}
// 		dnsres = res
// 		count++
// 		if time.Now().Sub(start) < 300*time.Millisecond {
// 			continue
// 		}
// 		log.Printf("####%d res in %v", count, time.Now().Sub(start))

// 		break
// 	}
// 	if nil != dnsres {
// 		w.WriteMsg(dnsres)
// 	}
// }

// func startLocalDNS(addr string) error {
// 	return dns.ListenAndServe(addr, "udp4", dns.HandlerFunc(proxyDNS))
// }

package protector

import (
	"crypto/rand"
	"errors"
	"log"
	"math/big"
	"net"
	"time"

	"github.com/miekg/dns"
)

type DNSRecord struct {
	IP       net.IP
	ExpireAt time.Time
}

type DnsResponse struct {
	records []DNSRecord
}

// PickRandomIP picks a random IP address from a DNS response
func (response *DnsResponse) PickRandomIP() (net.IP, error) {
	length := int64(len(response.records))
	if length < 1 {
		return nil, errors.New("no IP address")
	}

	index, err := rand.Int(rand.Reader, big.NewInt(length))
	if err != nil {
		return nil, err
	}

	record := response.records[index.Int64()]
	return record.IP, nil
}

func (response *DnsResponse) PickRecord() (*DNSRecord, error) {
	length := int64(len(response.records))
	if length < 1 {
		return nil, errors.New("no IP address")
	}
	return &response.records[0], nil
}

// dnsLookup is used whenever we need to conduct a DNS query over a given TCP connection
func DnsLookup(addr string, conn net.Conn) (*DnsResponse, error) {
	//log.Printf("Doing a DNS lookup on %s", addr)

	dnsResponse := &DnsResponse{
		records: make([]DNSRecord, 0),
	}

	// create the connection to the DNS server
	dnsConn := &dns.Conn{Conn: conn}
	defer dnsConn.Close()

	m := new(dns.Msg)
	m.Id = dns.Id()
	// set the question section in the dns query
	// Fqdn returns the fully qualified domain name
	m.SetQuestion(dns.Fqdn(addr), dns.TypeA)
	m.RecursionDesired = true

	dnsConn.WriteMsg(m)

	response, err := dnsConn.ReadMsg()
	if err != nil {
		log.Printf("Could not process DNS response: %v", err)
		return nil, err
	}
	now := time.Now()
	// iterate over RRs containing the DNS answer
	for _, answer := range response.Answer {
		if a, ok := answer.(*dns.A); ok {
			// append the result to our list of records
			// the A records in the RDATA section of the DNS answer
			// contains the actual IP address
			dnsResponse.records = append(dnsResponse.records,
				DNSRecord{
					IP:       a.A,
					ExpireAt: now.Add(time.Duration(a.Hdr.Ttl) * time.Second),
				})
			//log.Printf("###TTL:%d", a.Hdr.Ttl)
		}
	}
	return dnsResponse, nil
}

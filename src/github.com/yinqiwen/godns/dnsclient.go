// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godns

import (
	"errors"
	"io"
	"math/rand"
	"net"
	"sort"
	"strings"
	"time"
)

// DNSError represents a DNS lookup error.
type DNSError struct {
	Err       string // description of the error
	Name      string // name looked for
	Server    string // server used
	IsTimeout bool
}

func (e *DNSError) Error() string {
	if e == nil {
		return "<nil>"
	}
	s := "lookup " + e.Name
	if e.Server != "" {
		s += " on " + e.Server
	}
	s += ": " + e.Err
	return s
}

func (e *DNSError) Timeout() bool   { return e.IsTimeout }
func (e *DNSError) Temporary() bool { return e.IsTimeout }

const noSuchHost = "no such host"

// reverseaddr returns the in-addr.arpa. or ip6.arpa. hostname of the IP
// address addr suitable for rDNS (PTR) record lookup or an error if it fails
// to parse the IP address.
//func reverseaddr(addr string) (arpa string, err error) {
//	ip := net.ParseIP(addr)
//	if ip == nil {
//		return "", &DNSError{Err: "unrecognized address", Name: addr}
//	}
//	if ip.To4() != nil {
//		return itoa(int(ip[15])) + "." + itoa(int(ip[14])) + "." + itoa(int(ip[13])) + "." +
//			itoa(int(ip[12])) + ".in-addr.arpa.", nil
//	}
//	// Must be IPv6
//	buf := make([]byte, 0, len(ip)*4+len("ip6.arpa."))
//	// Add it, in reverse, to the buffer
//	for i := len(ip) - 1; i >= 0; i-- {
//		v := ip[i]
//		buf = append(buf, hexDigit[v&0xF])
//		buf = append(buf, '.')
//		buf = append(buf, hexDigit[v>>4])
//		buf = append(buf, '.')
//	}
//	// Append "ip6.arpa." and return (buf already has the final .)
//	buf = append(buf, "ip6.arpa."...)
//	return string(buf), nil
//}

// Find answer for name in dns message.
// On return, if err == nil, addrs != nil.
func answer(name, server string, dns *dnsMsg, qtype uint16) (cname string, addrs []dnsRR, err error) {
	addrs = make([]dnsRR, 0, len(dns.answer))

	if dns.rcode == dnsRcodeNameError && dns.recursion_available {
		return "", nil, &DNSError{Err: noSuchHost, Name: name}
	}
	if dns.rcode != dnsRcodeSuccess {
		// None of the error codes make sense
		// for the query we sent.  If we didn't get
		// a name error and we didn't get success,
		// the server is behaving incorrectly.
		return "", nil, &DNSError{Err: "server misbehaving", Name: name, Server: server}
	}

	// Look for the name.
	// Presotto says it's okay to assume that servers listed in
	// /etc/resolv.conf are recursive resolvers.
	// We asked for recursion, so it should have included
	// all the answers we need in this one packet.
Cname:
	for cnameloop := 0; cnameloop < 10; cnameloop++ {
		addrs = addrs[0:0]
		for _, rr := range dns.answer {
			if _, justHeader := rr.(*dnsRR_Header); justHeader {
				// Corrupt record: we only have a
				// header. That header might say it's
				// of type qtype, but we don't
				// actually have it. Skip.
				continue
			}
			h := rr.Header()
			if h.Class == dnsClassINET && h.Name == name {
				switch h.Rrtype {
				case qtype:
					addrs = append(addrs, rr)
				case dnsTypeCNAME:
					// redirect to cname
					name = rr.(*dnsRR_CNAME).Cname
					continue Cname
				}
			}
		}
		if len(addrs) == 0 {
			return "", nil, &DNSError{Err: noSuchHost, Name: name, Server: server}
		}
		return name, addrs, nil
	}

	return "", nil, &DNSError{Err: "too many redirects", Name: name, Server: server}
}

func isDomainName(s string) bool {
	// See RFC 1035, RFC 3696.
	if len(s) == 0 {
		return false
	}
	if len(s) > 255 {
		return false
	}
	if s[len(s)-1] != '.' { // simplify checking loop: make name end in dot
		s += "."
	}

	last := byte('.')
	ok := false // ok once we've seen a letter
	partlen := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		default:
			return false
		case 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c == '_':
			ok = true
			partlen++
		case '0' <= c && c <= '9':
			// fine
			partlen++
		case c == '-':
			// byte before dash cannot be dot
			if last == '.' {
				return false
			}
			partlen++
		case c == '.':
			// byte before dot cannot be dot, dash
			if last == '.' || last == '-' {
				return false
			}
			if partlen > 63 || partlen == 0 {
				return false
			}
			partlen = 0
		}
		last = c
	}

	return ok
}

// An SRV represents a single DNS SRV record.
type SRV struct {
	Target   string
	Port     uint16
	Priority uint16
	Weight   uint16
}

// byPriorityWeight sorts SRV records by ascending priority and weight.
type byPriorityWeight []*SRV

func (s byPriorityWeight) Len() int { return len(s) }

func (s byPriorityWeight) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s byPriorityWeight) Less(i, j int) bool {
	return s[i].Priority < s[j].Priority ||
		(s[i].Priority == s[j].Priority && s[i].Weight < s[j].Weight)
}

// shuffleByWeight shuffles SRV records by weight using the algorithm
// described in RFC 2782.  
func (addrs byPriorityWeight) shuffleByWeight() {
	sum := 0
	for _, addr := range addrs {
		sum += int(addr.Weight)
	}
	for sum > 0 && len(addrs) > 1 {
		s := 0
		n := rand.Intn(sum + 1)
		for i := range addrs {
			s += int(addrs[i].Weight)
			if s >= n {
				if i > 0 {
					t := addrs[i]
					copy(addrs[1:i+1], addrs[0:i])
					addrs[0] = t
				}
				break
			}
		}
		sum -= int(addrs[0].Weight)
		addrs = addrs[1:]
	}
}

// sort reorders SRV records as specified in RFC 2782.
func (addrs byPriorityWeight) sort() {
	sort.Sort(addrs)
	i := 0
	for j := 1; j < len(addrs); j++ {
		if addrs[i].Priority != addrs[j].Priority {
			addrs[i:j].shuffleByWeight()
			i = j
		}
	}
	addrs[i:].shuffleByWeight()
}

// An MX represents a single DNS MX record.
type MX struct {
	Host string
	Pref uint16
}

// byPref implements sort.Interface to sort MX records by preference
type byPref []*MX

func (s byPref) Len() int { return len(s) }

func (s byPref) Less(i, j int) bool { return s[i].Pref < s[j].Pref }

func (s byPref) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// sort reorders MX records as specified in RFC 5321.
func (s byPref) sort() {
	for i := range s {
		j := rand.Intn(i + 1)
		s[i], s[j] = s[j], s[i]
	}
	sort.Sort(s)
}

func net_read(c net.Conn, net string, buf []byte) (n int, err error) {
	switch net {
	case "tcp", "tcp4", "tcp6":
		n, err = c.Read(buf[0:2])
		if err != nil || n != 2 {
			return n, err
		}
		l, _ := unpackUint16(buf[0:2], 0)
		if l == 0 {
			return 0, errors.New("ErrShortRead")
		}
		if int(l) > len(buf) {
			return int(l), io.ErrShortBuffer
		}
		n, err = c.Read(buf[:l])
		if err != nil {
			return n, err
		}
		i := n
		for i < int(l) {
			j, err := c.Read(buf[i:int(l)])
			if err != nil {
				return i, err
			}
			i += j
		}
		n = i
		return
	case "", "udp", "udp4", "udp6":
		return c.Read(buf)
	}
	return -1, errors.New("Unexpected here")
}

func unpackUint16(msg []byte, off int) (v uint16, off1 int) {
	v = uint16(msg[off])<<8 | uint16(msg[off+1])
	off1 = off + 2
	return
}

// Helper function for packing
func packUint16(i uint16) (byte, byte) {
	return byte(i >> 8), byte(i)
}

func net_write(c net.Conn, net string, p []byte) (n int, err error) {
	switch net {
	case "tcp", "tcp4", "tcp6":
		if len(p) < 2 {
			return 0, io.ErrShortBuffer
		}
		a, b := packUint16(uint16(len(p)))
		n, err = c.Write([]byte{a, b})
		if err != nil {
			return n, err
		}
		if n != 2 {
			return n, io.ErrShortWrite
		}
		n, err = c.Write(p)
		if err != nil {
			return n, err
		}
		i := n
		if i < len(p) {
			j, err := c.Write(p[i:len(p)])
			if err != nil {
				return i, err
			}
			i += j
		}
		n = i
		return
	case "", "udp", "udp4", "udp6":
	    
		return c.Write(p)
	}
	return -1, errors.New("Unexpected here")
}

// Send a request on the connection and hope for a reply.
// Up to cfg.attempts attempts.
func exchange(cfg *dnsConfig, c net.Conn, name string, qtype uint16) (*dnsMsg, error) {
	if len(name) >= 256 {
		return nil, &DNSError{Err: "name too long", Name: name}
	}
	out := new(dnsMsg)
	out.id = uint16(rand.Int()) ^ uint16(time.Now().UnixNano())
	out.question = []dnsQuestion{
		{name, qtype, dnsClassINET},
	}
	out.recursion_desired = true
	msg, ok := out.Pack()
	if !ok {
		return nil, &DNSError{Err: "internal error - cannot pack message", Name: name}
	}

	for attempt := 0; attempt < cfg.attempts; attempt++ {
		//n, err := c.Write(msg)
		n, err := net_write(c, cfg.net, msg)
		if err != nil {
			return nil, err
		}

		if cfg.timeout == 0 {
			c.SetReadDeadline(time.Time{})
		} else {
			c.SetReadDeadline(time.Now().Add(time.Duration(cfg.timeout) * time.Second))
		}
		package_size := 2000
		if strings.EqualFold(cfg.net, "tcp") || strings.EqualFold(cfg.net, "tcpv4") || strings.EqualFold(cfg.net, "tcpv6") {
			package_size = 65536
		}
		buf := make([]byte, package_size) // More than enough.
		//n, err = c.Read(buf)
		n, err = net_read(c, cfg.net, buf)
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Timeout() {
				continue
			}
			return nil, err
		}
		buf = buf[0:n]
		in := new(dnsMsg)
		if !in.Unpack(buf) || in.id != out.id {
			continue
		}
		return in, nil
	}
	var server string
	if a := c.RemoteAddr(); a != nil {
		server = a.String()
	}
	return nil, &DNSError{Err: "no answer from server", Name: name, Server: server, IsTimeout: true}
}

// Do a lookup for a single name, which must be rooted
// (otherwise answer will not find the answers).
func tryOneName(cfg *dnsConfig, name string, qtype uint16) (cname string, addrs []dnsRR, err error) {
	if len(cfg.servers) == 0 {
		return "", nil, &DNSError{Err: "no DNS servers", Name: name}
	}
	for i := 0; i < len(cfg.servers); i++ {
		// Calling Dial here is scary -- we have to be sure
		// not to dial a name that will require a DNS lookup,
		// or Dial will call back here to translate it.
		// The DNS config parser has already checked that
		// all the cfg.servers[i] are IP addresses, which
		// Dial will use without a DNS lookup.
		server := cfg.servers[i] + ":53"
		if len(cfg.net) == 0 {
			cfg.net = "udp"
		}
		dialTimeout := net.DialTimeout
		if nil != cfg.dialTimeout {
			dialTimeout = cfg.dialTimeout
		}
		c, cerr := dialTimeout(cfg.net, server, time.Duration(cfg.timeout)*time.Second)
		if cerr != nil {
			err = cerr
			continue
		}

		msg, merr := exchange(cfg, c, name, qtype)
		c.Close()
		if merr != nil {
			err = merr
			continue
		}
		cname, addrs, err = answer(name, server, msg, qtype)
		if err == nil || err.(*DNSError).Err == noSuchHost {
			break
		}
	}
	return
}

func convertRR_A(records []dnsRR) []net.IP {
	addrs := make([]net.IP, len(records))
	for i, rr := range records {
		a := rr.(*dnsRR_A).A
		addrs[i] = net.IPv4(byte(a>>24), byte(a>>16), byte(a>>8), byte(a))
	}
	return addrs
}

func convertRR_AAAA(records []dnsRR) []net.IP {
	addrs := make([]net.IP, len(records))
	for i, rr := range records {
		a := make(net.IP, net.IPv6len)
		copy(a, rr.(*dnsRR_AAAA).AAAA[:])
		addrs[i] = a
	}
	return addrs
}

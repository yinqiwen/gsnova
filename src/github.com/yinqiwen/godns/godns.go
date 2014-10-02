package godns

import (
	"errors"
	"net"
	"sync"
	"time"
)

const (
	DNS_CACHE_TTL_SELF    = -2
	DNS_CACHE_TTL_FOREVER = -1
	DNS_NOCACHE           = 0
)

type LookupOptions struct {
	DNSServers  []string // DNS servers to use
	CacheTTL    int      //
	Net         string   //Default:udp
	OnlyIPv4    bool
	DialTimeout func(net, addr string, timeout time.Duration) (net.Conn, error)
}

type dnsCache struct {
	ips []net.IP
	ttl uint32
	ts  time.Time
}

var cacheLookupResults = make(map[string]*dnsCache)
var cacheMutex sync.Mutex

func lookup(cfg *dnsConfig, name string, qtype uint16) (cname string, addrs []dnsRR, err error) {
	if !isDomainName(name) {
		return name, nil, &DNSError{Err: "invalid domain name", Name: name}
	}
	//onceLoadConfig.Do(loadConfig)
	if cfg == nil {
		err = errors.New("No DNS config set")
		return
	}
	// If name is rooted (trailing dot) or has enough dots,
	// try it by itself first.
	rooted := len(name) > 0 && name[len(name)-1] == '.'
	if rooted || count(name, '.') >= cfg.ndots {
		rname := name
		if !rooted {
			rname += "."
		}
		// Can try as ordinary name.
		cname, addrs, err = tryOneName(cfg, rname, qtype)
		if err == nil {
			return
		}
	}
	if rooted {
		return
	}

	// Otherwise, try suffixes.
	for i := 0; i < len(cfg.search); i++ {
		rname := name + "." + cfg.search[i]
		if rname[len(rname)-1] != '.' {
			rname += "."
		}
		cname, addrs, err = tryOneName(cfg, rname, qtype)
		if err == nil {
			return
		}
	}

	// Last ditch effort: try unsuffixed.
	rname := name
	if !rooted {
		rname += "."
	}
	cname, addrs, err = tryOneName(cfg, rname, qtype)
	if err == nil {
		return
	}
	return
}

// goLookupHost is the native Go implementation of LookupHost.
// Used only if cgoLookupHost refuses to handle the request
// (that is, only if cgoLookupHost is the stub in cgo_stub.go).
// Normally we let cgo use the C library resolver instead of
// depending on our lookup code, so that Go and C get the same
// answers.
func LookupHost(name string, options *LookupOptions) (addrs []string, err error) {
	if nil == options || nil == options.DNSServers || len(options.DNSServers) == 0 {
		return net.LookupHost(name)
	}
	ips, err := LookupIP(name, options)
	if err != nil {
		return
	}
	addrs = make([]string, 0, len(ips))
	for _, ip := range ips {
		addrs = append(addrs, ip.String())
	}
	return
}

// goLookupIP is the native Go implementation of LookupIP.
// Used only if cgoLookupIP refuses to handle the request
// (that is, only if cgoLookupIP is the stub in cgo_stub.go).
// Normally we let cgo use the C library resolver instead of
// depending on our lookup code, so that Go and C get the same
// answers.
func LookupIP(name string, options *LookupOptions) (addrs []net.IP, err error) {
	if options.CacheTTL != DNS_NOCACHE {
		cacheMutex.Lock()
		result, exist := cacheLookupResults[name]
		cacheMutex.Unlock()
		if exist {
			if options.CacheTTL == DNS_CACHE_TTL_FOREVER || (time.Now().Before(result.ts.Add(time.Duration(result.ttl) * time.Second))) {
				return result.ips, nil
			}
		}
	}

	if nil == options || nil == options.DNSServers || len(options.DNSServers) == 0 {
		return net.LookupIP(name)
	}

	haddrs := lookupStaticHost(name)
	if len(haddrs) > 0 {
		for _, haddr := range haddrs {
			if ip := net.ParseIP(haddr); ip != nil {
				addrs = append(addrs, ip)
			}
		}
		if len(addrs) > 0 {
			return
		}
	}

	dnscfg, dnserr := dnsConfigWithOptions(options)
	if dnserr != nil || dnscfg == nil {
		err = dnserr
		return
	}
	var records []dnsRR
	var cname string
	ttl := uint32(3600)
	cname, records, err = lookup(dnscfg, name, dnsTypeA)
	if err != nil {
		return
	}
	if options.CacheTTL > 0 {
		ttl = uint32(options.CacheTTL)
	}
	if len(records) > 0 && nil != records[0].Header() {
		if options.CacheTTL == DNS_CACHE_TTL_SELF {
			ttl = records[0].Header().Ttl
		}
		if records[0].Header().Ttl > ttl {
			ttl = records[0].Header().Ttl
		}
	}

	addrs = convertRR_A(records)
	if cname != "" {
		name = cname
	}

	if !options.OnlyIPv4 {
		_, records, err = lookup(dnscfg, name, dnsTypeAAAA)
		if err != nil && len(addrs) > 0 {
			// Ignore error because A lookup succeeded.
			err = nil
		}
		if err != nil {
			return
		}
		addrs = append(addrs, convertRR_AAAA(records)...)
	}

	if options.CacheTTL != DNS_NOCACHE {
		cacheMutex.Lock()
		cacheLookupResults[name] = &dnsCache{addrs, ttl, time.Now()}
		cacheMutex.Unlock()
	}
	return
}

// goLookupCNAME is the native Go implementation of LookupCNAME.
// Used only if cgoLookupCNAME refuses to handle the request
// (that is, only if cgoLookupCNAME is the stub in cgo_stub.go).
// Normally we let cgo use the C library resolver instead of
// depending on our lookup code, so that Go and C get the same
// answers.
func LookupCNAME(name string, options *LookupOptions) (cname string, err error) {
	if nil == options || nil == options.DNSServers || len(options.DNSServers) == 0 {
		return net.LookupCNAME(name)
	}

	dnscfg, dnserr := dnsConfigWithOptions(options)
	if dnserr != nil || dnscfg == nil {
		err = dnserr
		return
	}
	_, rr, err := lookup(dnscfg, name, dnsTypeCNAME)
	if err != nil {
		return
	}
	cname = rr[0].(*dnsRR_CNAME).Cname
	return
}

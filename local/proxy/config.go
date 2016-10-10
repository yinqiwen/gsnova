package proxy

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/yinqiwen/gsnova/common/gfwlist"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/local/hosts"
)

var GConf LocalConfig
var mygfwlist *gfwlist.GFWList
var cnIPRange *IPRangeHolder

const (
	BlockedByGFWRule = "BlockedByGFW"
	InHostsRule      = "InHosts"
	IsCNIPRule       = "IsCNIP"
)

func matchHostnames(pattern, host string) bool {
	host = strings.TrimSuffix(host, ".")
	pattern = strings.TrimSuffix(pattern, ".")

	if len(pattern) == 0 || len(host) == 0 {
		return false
	}

	patternParts := strings.Split(pattern, ".")
	hostParts := strings.Split(host, ".")

	if len(patternParts) != len(hostParts) {
		return false
	}

	for i, patternPart := range patternParts {
		if i == 0 && patternPart == "*" {
			continue
		}
		if patternPart != hostParts[i] {
			return false
		}
	}
	return true
}

type ProxyChannelConfig struct {
	Enable              bool
	Name                string
	Type                string
	ServerList          []string
	ConnsPerServer      int
	SNI                 []string
	SNIProxy            string
	Proxy               string
	DialTimeout         int
	ReadTimeout         int
	ReconnectPeriod     int
	HeartBeatPeriod     int
	RCPRandomAdjustment int
	HTTPChunkPushEnable bool
	ForceTLS            bool

	proxyURL *url.URL
}

func (c *ProxyChannelConfig) IsDirect() bool {
	return c.Type == "DIRECT"
}

func (c *ProxyChannelConfig) ProxyURL() *url.URL {
	if nil != c.proxyURL {
		return c.proxyURL
	}
	if len(c.Proxy) > 0 {
		var err error
		c.proxyURL, err = url.Parse(c.Proxy)
		if nil != err {
			log.Printf("Failed to parse proxy URL ", c.Proxy)
		}
	}
	return c.proxyURL
}

type PACConfig struct {
	Method   []string
	Host     []string
	URL      []string
	Rule     []string
	Protocol []string
	Remote   string
}

func (pac *PACConfig) ruleInHosts(req *http.Request) bool {
	return hosts.InHosts(req.Host)
}

func (pac *PACConfig) matchProtocol(protocol string) bool {
	if len(pac.Protocol) == 0 {
		return true
	}
	for _, p := range pac.Protocol {
		if p == "*" || strings.EqualFold(p, protocol) {
			return true
		}
	}
	return false
}

func (pac *PACConfig) matchRules(ip string, req *http.Request) bool {
	if len(pac.Rule) == 0 {
		return true
	}

	ok := true

	for _, rule := range pac.Rule {
		not := false
		if strings.HasPrefix(rule, "!") {
			not = true
			rule = rule[1:]
		}
		if strings.EqualFold(rule, InHostsRule) {
			if nil == req {
				ok = false
			} else {
				ok = pac.ruleInHosts(req)
			}
		} else if strings.EqualFold(rule, BlockedByGFWRule) {
			if nil != mygfwlist && nil != req {
				ok = mygfwlist.IsBlockedByGFW(req)
				if !ok {
					log.Printf("#### %s is NOT BlockedByGFW", req.Host)
				}
			} else {
				ok = true
				log.Printf("NIL GFWList object or request")
			}
		} else if strings.EqualFold(rule, IsCNIPRule) {
			if len(ip) == 0 || nil == cnIPRange {
				log.Printf("NIL CNIP content  or IP/Domain")
				ok = false
			} else {
				var err error
				if net.ParseIP(ip) == nil {
					ip, err = DnsGetDoaminIP(ip)
				}
				if nil == err {
					_, err = cnIPRange.FindCountry(ip)
				}
				ok = (nil == err)
				log.Printf("ip:%s is CNIP:%v", ip, ok)
			}

		} else {
			log.Printf("###Invalid rule:%s", rule)
		}
		if not {
			ok = ok != true
		}
		if !ok {
			break
		}
	}
	return ok
}

func MatchPatterns(str string, rules []string) bool {
	if len(rules) == 0 {
		return true
	}
	str = strings.ToLower(str)
	for _, pattern := range rules {
		matched, err := filepath.Match(pattern, str)
		if nil != err {
			log.Printf("Invalid pattern:%s with reason:%v", pattern, err)
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

func (pac *PACConfig) Match(protocol string, ip string, req *http.Request) bool {
	ret := pac.matchProtocol(protocol)
	if !ret {
		return false
	}
	ret = pac.matchRules(ip, req)
	if !ret {
		return false
	}
	if nil == req {
		if len(pac.Host) > 0 || len(pac.Method) > 0 || len(pac.URL) > 0 {
			return false
		}
		return true
	}
	host := req.Host
	if len(pac.Host) > 0 && strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}
	return MatchPatterns(host, pac.Host) && MatchPatterns(req.Method, pac.Method) && MatchPatterns(req.URL.String(), pac.URL)
}

type ProxyConfig struct {
	Local    string
	PAC      []PACConfig
	SNISniff bool
}

func (cfg *ProxyConfig) findProxyByRequest(proto string, ip string, req *http.Request) Proxy {
	var p Proxy
	if len(ip) > 0 && helper.IsPrivateIP(ip) {
		p = getProxyByName("Direct")
		if nil != p {
			return p
		}
	}
	for _, pac := range cfg.PAC {
		if pac.Match(proto, ip, req) {
			p = getProxyByName(pac.Remote)
			break
		}

	}
	if nil == p {
		log.Printf("No proxy found.")
	}
	return p
}

type EncryptConfig struct {
	Method string
	Key    string
}

type LocalDNSConfig struct {
	Listen     string
	TrustedDNS []string
	FastDNS    []string
	TCPConnect bool
	CacheSize  int
}

type AdminConfig struct {
	Listen    string
	ConfigDir string
}

type GFWListConfig struct {
	URL      string
	UserRule []string
	Proxy    string
}

type LocalConfig struct {
	Log              []string
	Encrypt          EncryptConfig
	UserAgent        string
	Auth             string
	LocalDNS         LocalDNSConfig
	UDPGWAddr        string
	ChannelKeepAlive bool
	Admin            AdminConfig
	GFWList          GFWListConfig
	Proxy            []ProxyConfig
	Channel          []ProxyChannelConfig
}

func (cfg *LocalConfig) init() error {
	forwardProxies := make(map[string]bool)
	for _, pcfg := range cfg.Proxy {
		for _, pac := range pcfg.PAC {
			if strings.Contains(pac.Remote, "://") {
				forwardProxies[pac.Remote] = true
			}
		}
	}
	for forwardProxy, _ := range forwardProxies {
		forwardChannel := ProxyChannelConfig{
			Enable: true,
			Name:   forwardProxy,
			Type:   "direct",
			Proxy:  forwardProxy,
		}
		cfg.Channel = append(cfg.Channel, forwardChannel)
	}

	gfwlistEnable := false
	cnIPEnable := false
	for i, _ := range cfg.Proxy {
		for j, _ := range cfg.Proxy[i].PAC {
			rules := cfg.Proxy[i].PAC[j].Rule
			for _, r := range rules {
				if strings.Contains(r, BlockedByGFWRule) || strings.Contains(r, IsCNIPRule) {
					gfwlistEnable = true
				}
				if strings.Contains(r, IsCNIPRule) {
					cnIPEnable = true
				}
			}
		}
	}

	if gfwlistEnable {
		go func() {
			hc, _ := NewHTTPClient(&ProxyChannelConfig{Proxy: cfg.GFWList.Proxy})
			dst := proxyHome + "/gfwlist.txt"
			tmp, err := gfwlist.NewGFWList(cfg.GFWList.URL, hc, cfg.GFWList.UserRule, dst, true)
			if nil == err {
				mygfwlist = tmp
			} else {
				log.Printf("[ERROR]Failed to create gfwlist  for reason:%v", err)
			}
		}()
	}
	if cnIPEnable {
		go func() {
			iprangeFile := proxyHome + "/" + cnIPFile
			ipHolder, err := parseApnicIPFile(iprangeFile)
			nextFetchTime := 1 * time.Second
			if nil == err {
				cnIPRange = ipHolder
				nextFetchTime = 1 * time.Minute
			}
			var hc *http.Client
			for {
				select {
				case <-time.After(nextFetchTime):
					if nil == hc {
						hc, err = NewHTTPClient(&ProxyChannelConfig{})
					}
					if nil != hc {
						ipHolder, err = getCNIPRangeHolder(hc)
						if nil != err {
							log.Printf("[ERROR]Failed to fetch CNIP file:%v", err)
							nextFetchTime = 1 * time.Second
						} else {
							nextFetchTime = 24 * time.Hour
							cnIPRange = ipHolder
						}
					}
				}
			}
		}()
	}
	return nil
}

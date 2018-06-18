package local

import (
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/dns"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/hosts"
	"github.com/yinqiwen/gsnova/common/logger"
)

var GConf LocalConfig

const (
	BlockedByGFWRule = "BlockedByGFW"
	InHostsRule      = "InHosts"
	IsCNIPRule       = "IsCNIP"
	IsPrivateIPRule  = "IsPrivateIP"
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
			gfwList := getGFWList()
			if nil != gfwList && nil != req {
				ok = gfwList.IsBlockedByGFW(req)
				if !ok {
					logger.Debug("#### %s is NOT BlockedByGFW", req.Host)
				}
			} else {
				ok = true
				logger.Debug("NIL GFWList object or request")
			}
		} else if strings.EqualFold(rule, IsCNIPRule) {
			if len(ip) == 0 || nil == dns.CNIPSet {
				logger.Debug("NIL CNIP content  or IP/Domain")
				ok = false
			} else {
				var err error
				if net.ParseIP(ip) == nil {
					ip, err = dns.DnsGetDoaminIP(ip)
				}
				if nil == err {
					ok = dns.CNIPSet.IsInCountry(net.ParseIP(ip), "CN")
				}
				logger.Debug("ip:%s is CNIP:%v", ip, ok)
			}
		} else if strings.EqualFold(rule, IsPrivateIPRule) {
			if len(ip) == 0 {
				ok = false
			} else {
				ok = helper.IsPrivateIP(ip)
			}
		} else {
			logger.Error("###Invalid rule:%s", rule)
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
			logger.Error("Invalid pattern:%s with reason:%v", pattern, err)
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

type HTTPDumpConfig struct {
	Dump        string
	Domain      []string
	ExcludeBody []string
	IncludeBody []string
}

func (dump *HTTPDumpConfig) MatchDomain(host string) bool {
	for _, dm := range dump.Domain {
		matched, _ := filepath.Match(dm, host)
		if matched {
			return true
		}
	}
	return false
}

type ProxyConfig struct {
	Local       string
	Forward     string
	MITM        bool //Man-in-the-middle
	Transparent bool
	HTTPDump    HTTPDumpConfig
	PAC         []PACConfig
}

func (cfg *ProxyConfig) getProxyChannelByHost(proto string, host string) string {
	creq, _ := http.NewRequest("Connect", "https://"+host, nil)
	return cfg.findProxyChannelByRequest(proto, host, creq)
}

func (cfg *ProxyConfig) findProxyChannelByRequest(proto string, ip string, req *http.Request) string {
	var channelName string
	// if len(ip) > 0 && helper.IsPrivateIP(ip) {
	// 	//channel = "direct"
	// 	return channel.DirectChannelName
	// }
	for _, pac := range cfg.PAC {
		if pac.Match(proto, ip, req) {
			channelName = pac.Remote
			break
		}
	}
	if len(channelName) == 0 {
		logger.Error("No proxy channel found.")
	}
	return channelName
}

type AdminConfig struct {
	Listen        string
	BroadcastAddr string
	ConfigDir     string
}

type UDPGWConfig struct {
	Addr string
}

type SNIConfig struct {
	Redirect map[string]string
}

func (sni *SNIConfig) redirect(domain string) (string, bool) {
	for k, v := range sni.Redirect {
		matched, err := filepath.Match(k, domain)
		if nil != err {
			logger.Error("Invalid pattern:%s with reason:%v", k, err)
			continue
		}
		if matched {
			return v, true
		}
	}
	return "", false
}

type GFWListConfig struct {
	URL                   string
	UserRule              []string
	Proxy                 string
	RefershPeriodMiniutes int
}

type LocalConfig struct {
	Log             []string
	Cipher          channel.CipherConfig
	Mux             channel.MuxConfig
	ProxyLimit      channel.ProxyLimitConfig
	UserAgent       string
	UPNPExposePort  int
	LocalDNS        dns.LocalDNSConfig
	UDPGW           UDPGWConfig
	SNI             SNIConfig
	Admin           AdminConfig
	GFWList         GFWListConfig
	TransparentMark int
	Proxy           []ProxyConfig
	Channel         []channel.ProxyChannelConfig
}

func (cfg *LocalConfig) init() error {
	haveDirect := false
	for i := range GConf.Channel {
		if GConf.Channel[i].Name == channel.DirectChannelName && GConf.Channel[i].Enable {
			haveDirect = true
			GConf.Channel[i].ServerList = []string{"direct://0.0.0.0:0"}
			GConf.Channel[i].ConnsPerServer = 1
		}
		if len(GConf.Channel[i].Cipher.Key) == 0 {
			GConf.Channel[i].Cipher = GConf.Cipher
		}
		if len(GConf.Channel[i].HTTP.UserAgent) == 0 {
			GConf.Channel[i].HTTP.UserAgent = GConf.UserAgent
		}
		GConf.Channel[i].Adjust()
	}

	if !haveDirect {
		directProxyChannel := make([]channel.ProxyChannelConfig, 1)
		directProxyChannel[0].Name = channel.DirectChannelName
		directProxyChannel[0].Enable = true
		directProxyChannel[0].ConnsPerServer = 1
		directProxyChannel[0].LocalDialMSTimeout = 5000
		directProxyChannel[0].ServerList = []string{"direct://0.0.0.0:0"}
		GConf.Channel = append(directProxyChannel, GConf.Channel...)
	}
	return nil
}

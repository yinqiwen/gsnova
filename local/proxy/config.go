package proxy

import (
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/yinqiwen/gsnova/common/gfwlist"
	"github.com/yinqiwen/gsnova/local/hosts"
)

var GConf LocalConfig
var mygfwlist *gfwlist.GFWList

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
	HTTPProxy           string
	DialTimeout         int
	ReadTimeout         int
	ReconnectPeriod     int
	HeartBeatPeriod     int
	RCPRandomAdjustment int
	HTTPChunkPushEnable bool
	ForceTLS            bool
}

func (c *ProxyChannelConfig) IsDirect() bool {
	return c.Type == "DIRECT"
}

// type PAASConfig struct {
// 	Enable                  bool
// 	ServerList              []string
// 	ConnsPerServer          int
// 	SNI                     string
// 	SNIProxy                string
// 	HTTPProxy               string
// 	DialTimeout             int
// 	HTTPReadTimeout         int
// 	WSReadTimeout           int
// 	WSReconnectPeriod       int
// 	WSHeartBeatPeriod       int
// 	WSRCPRandomAdjustment   int
// 	HTTPRCPRandomAdjustment int
// 	HTTPReconnectPeriod     int
// 	HTTPChunkPushEnable     bool
// }

// type GAEConfig struct {
// 	Enable         bool
// 	ServerList     []string
// 	SNI            []string
// 	InjectRange    []string
// 	ConnsPerServer int
// 	HTTPProxy      string
// 	DialTimeout    int
// 	ReadTimeout    int
// }

// type VPSConfig struct {
// 	Enable              bool
// 	Server              string
// 	ConnsPerServer      int
// 	HTTPProxy           string
// 	DialTimeout         int
// 	ReadTimeout         int
// 	ReconnectPeriod     int
// 	HeartBeatPeriod     int
// 	RCPRandomAdjustment int
// }

type PACConfig struct {
	Method   []string
	Host     []string
	Path     []string
	Rule     []string
	Protocol []string
	Remote   string

	methodRegex []*regexp.Regexp
	hostRegex   []*regexp.Regexp
	pathRegex   []*regexp.Regexp
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
		if strings.EqualFold(rule, "InHosts") {
			if nil == req {
				ok = false
			} else {
				ok = pac.ruleInHosts(req)
			}
		} else if strings.EqualFold(rule, "BlockedByGFW") {
			if nil != mygfwlist && nil != req {
				ok = mygfwlist.IsBlockedByGFW(req)
			} else {
				log.Printf("NIL GFWList object or request")
			}
		} else if strings.EqualFold(rule, "IsCNIP") {
			if len(ip) == 0 {
				ok = false
			} else {
				if net.ParseIP(ip) == nil {
					ok = false
				} else {
					_, err := cnIPRange.FindCountry(ip)
					ok = (nil == err)
				}
			}
			log.Printf("ip:%s is CNIP:%v", ip, ok)
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

func MatchRegexs(str string, rules []*regexp.Regexp) bool {
	if len(rules) == 0 {
		return true
	}
	str = strings.ToLower(str)
	for _, regex := range rules {
		if regex.MatchString(str) {
			return true
		}
	}
	return false
}
func NewRegex(rules []string) ([]*regexp.Regexp, error) {
	regexs := make([]*regexp.Regexp, 0)
	for _, originrule := range rules {
		if originrule == "*" && len(rules) == 1 {
			break
		}
		rule := strings.Replace(strings.ToLower(originrule), "*", ".*", -1)
		reg, err := regexp.Compile(rule)
		if nil != err {
			log.Printf("Invalid pattern:%s for reason:%v", originrule, err)
			return nil, err
		} else {
			regexs = append(regexs, reg)
		}
	}

	return regexs, nil
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
		if len(pac.hostRegex) > 0 || len(pac.methodRegex) > 0 || len(pac.pathRegex) > 0 {
			return false
		}
		return true
	}
	return MatchRegexs(req.Host, pac.hostRegex) && MatchRegexs(req.Method, pac.methodRegex) && MatchRegexs(req.URL.Path, pac.pathRegex)
}

type ProxyConfig struct {
	Local    string
	PAC      []PACConfig
	SNISniff bool
}

func (cfg *ProxyConfig) findProxyByRequest(proto string, ip string, req *http.Request) Proxy {
	var p Proxy
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

// type DirectConfig struct {
// 	SNI         []string
// 	DialTimeout int
// 	ReadTimeout int
// 	//SNIMapping  map[string]string
// }

type EncryptConfig struct {
	Method string
	Key    string
}

type LocalDNSConfig struct {
	Listen     string
	TrustedDNS []string
	TCPConnect bool
}

type AdminConfig struct {
	Listen    string
	ConfigDir string
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
	Proxy            []ProxyConfig
	Channel          []ProxyChannelConfig

	// PAAS   PAASConfig
	// GAE    GAEConfig
	// VPS    VPSConfig
	// Direct DirectConfig
}

func (cfg *LocalConfig) init() error {

	// gfwlist, err := gfwlist.NewGFWList("https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt", "", true)
	// if nil != err {
	// 	return err
	// }
	// mygfwlist = gfwlist
	return nil
}

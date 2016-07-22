package proxy

import (
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/yinqiwen/gsnova/local/hosts"
)

var GConf LocalConfig

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

type PAASConfig struct {
	Enable         bool
	ServerList     []string
	ConnsPerServer int
	SNIProxy       string
}

type GAEConfig struct {
	Enable         bool
	ServerList     []string
	TLSServerName  []string
	InjectRange    []string
	ConnsPerServer int
}

type PACConfig struct {
	Method []string
	Host   []string
	Path   []string
	Rule   []string
	Remote string

	methodRegex []*regexp.Regexp
	hostRegex   []*regexp.Regexp
	pathRegex   []*regexp.Regexp
}

func (pac *PACConfig) ruleInHosts(req *http.Request) bool {
	return hosts.InHosts(req.Host)
}

func (pac *PACConfig) matchRules(req *http.Request) bool {
	if len(pac.Rule) == 0 {
		return true
	}
	ok := true
	for _, rule := range pac.Rule {
		if strings.EqualFold(rule, "InHosts") {
			ok = pac.ruleInHosts(req)
			if !ok {
				break
			}
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

func (pac *PACConfig) Match(req *http.Request) bool {
	return pac.matchRules(req) && MatchRegexs(req.Host, pac.hostRegex) && MatchRegexs(req.Method, pac.methodRegex) && MatchRegexs(req.URL.Path, pac.pathRegex)
}

type ProxyConfig struct {
	Local string
	PAC   []PACConfig
}

type LocalConfig struct {
	Log       []string
	UserAgent string
	RC4Key    string
	Auth      string
	Proxy     []ProxyConfig
	PAAS      PAASConfig
	GAE       GAEConfig
}

func (cfg *LocalConfig) init() error {
	for i, _ := range cfg.Proxy {
		for j, pac := range cfg.Proxy[i].PAC {
			cfg.Proxy[i].PAC[j].methodRegex, _ = NewRegex(pac.Method)
			cfg.Proxy[i].PAC[j].hostRegex, _ = NewRegex(pac.Host)
			cfg.Proxy[i].PAC[j].pathRegex, _ = NewRegex(pac.Path)
		}
	}
	return nil
}

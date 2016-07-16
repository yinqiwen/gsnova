package proxy

import (
	"log"
	"net/http"
	"regexp"
	"strings"
)

var GConf LocalConfig

type PAASConfig struct {
	Enable         bool
	ServerList     []string
	ConnsPerServer int
}

type GAEConfig struct {
	Enable         bool
	ServerList     []string
	InjectRange    []string
	ConnsPerServer int
}

type PACConfig struct {
	Method []string
	Host   []string
	Path   []string
	Remote string

	methodRegex []*regexp.Regexp
	hostRegex   []*regexp.Regexp
	pathRegex   []*regexp.Regexp
}

func MatchRegexs(str string, rules []*regexp.Regexp) bool {
	if len(rules) == 0 {
		return true
	}
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
		rule := strings.Replace(originrule, "*", ".*", -1)
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
	return MatchRegexs(req.Host, pac.hostRegex) && MatchRegexs(req.Method, pac.methodRegex) && MatchRegexs(req.URL.Path, pac.pathRegex)
}

type ProxyConfig struct {
	Local string
	PAC   []PACConfig
}

type LocalConfig struct {
	Log       []string
	UserAgent string
	RC4Key    string
	User      string
	Proxy     []ProxyConfig
	PAAS      PAASConfig
	GAE       GAEConfig
}

func (cfg *LocalConfig) init() error {
	for _, p := range cfg.Proxy {
		for _, pac := range p.PAC {
			pac.methodRegex, _ = NewRegex(pac.Method)
			pac.hostRegex, _ = NewRegex(pac.Host)
			pac.pathRegex, _ = NewRegex(pac.Path)
		}
	}
	return nil
}

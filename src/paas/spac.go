package paas

import (
	"common"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type Rule struct {
	regexs []*regexp.Regexp
	proxy  string
}

type SpacConfig struct {
	defaultRule               string
	enableGoogleHttpDispatch  bool
	enableGoogleHttpsDispatch bool
	googleSites               []string
	rules                     []*Rule
	//ruleList                  map[string][]*regexp.Regexp
}

var spac *SpacConfig

var registedRemoteConnManager map[string]RemoteConnectionManager = make(map[string]RemoteConnectionManager)

func RegisteRemoteConnManager(connManager RemoteConnectionManager) {
	//connManager.Init()
	registedRemoteConnManager[connManager.GetName()] = connManager
}

func InitSpac() {
	spac = &SpacConfig{}
	spac.rules = make([]*Rule, 0)
	spac.defaultRule, _ = common.Cfg.GetProperty("SPAC", "DefaultRule")

	if sites, exist := common.Cfg.GetProperty("SPAC", "GoogleSites"); exist {
		spac.googleSites = strings.Split(sites, "|")
	}
	index := 0
	for {
		v, exist := common.Cfg.GetProperty("SPAC", "DispatchRule["+strconv.Itoa(index)+"]")
		if !exist || len(v) == 0 {
			break
		}
		vv := strings.Split(v, "->")
		if len(vv) == 2 {
			r := &Rule{}
			r.proxy = strings.ToUpper(strings.TrimSpace(vv[1]))

			rules := strings.Split(strings.TrimSpace(vv[0]), "|")
			for _, originrule := range rules {
				rule := strings.TrimSpace(originrule)
				rule = strings.Replace(rule, ".", "\\.", -1)
				rule = strings.Replace(rule, "*", ".*", -1)
				reg, err := regexp.Compile(rule)
				if nil != err {
					log.Printf("Invalid pattern:%s for reason:%s\n", originrule, err.Error())
				} else {
					r.regexs = append(r.regexs, reg)
				}
			}
			spac.rules = append(spac.rules, r)
		} else {
			log.Printf("Invalid diaptch rule:%s\n", v)
		}
		index++
	}

	if len(spac.defaultRule) == 0 {
		spac.defaultRule = GAE_NAME
	}
}

func SelectProxy(req *http.Request) (RemoteConnectionManager, bool) {
	url := req.RequestURI
	if strings.EqualFold(req.Method, "CONNECT") {
		url = strings.Split(url, ":")[0]
		url = "https://" + url
	}
	proxyName := spac.defaultRule
	matched := false
	for _, r := range spac.rules {
		for _, regex := range r.regexs {
			if regex.MatchString(url) {
				proxyName = r.proxy
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}
	v, ok := registedRemoteConnManager[proxyName]
	if !ok {
		log.Printf("No proxy:%s defined.\n", proxyName)
	}
	return v, ok
}

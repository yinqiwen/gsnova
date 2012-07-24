package paas

import (
	"common"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type SpacConfig struct {
	defaultRule               string
	enableGoogleHttpDispatch  bool
	enableGoogleHttpsDispatch bool
	googleSites               []string
	ruleTable                 map[string][]*regexp.Regexp
}

var spac *SpacConfig

var registedRemoteConnManager map[string]RemoteConnectionManager = make(map[string]RemoteConnectionManager)

func RegisteRemoteConnManager(connManager RemoteConnectionManager) {
	//connManager.Init()
	registedRemoteConnManager[connManager.GetName()] = connManager
}

func InitSpac() {
	spac = &SpacConfig{}
	spac.ruleTable = make(map[string][]*regexp.Regexp)
	spac.defaultRule, _ = common.Cfg.GetProperty("SPAC", "DefaultRule")
	spac.enableGoogleHttpDispatch, _ = common.Cfg.GetBoolProperty("SPAC", "GoogleHttpDispatchEnable")
	spac.enableGoogleHttpsDispatch, _ = common.Cfg.GetBoolProperty("SPAC", "GoogleHttpsDispatchEnable")
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
			key := strings.ToUpper(strings.TrimSpace(vv[1]))
			rules := strings.Split(strings.TrimSpace(vv[0]), "|")
			for _, originrule := range rules {
				rule := strings.TrimSpace(originrule)
				rule = strings.Replace(rule, ".", "\\.", -1)
				rule = strings.Replace(rule, "*", ".*", -1)
				reg, err := regexp.Compile(rule)
				if nil != err {
					log.Printf("Invalid pattern:%s for reason:%s\n", originrule, err.Error())
				} else {
					regs := spac.ruleTable[key]
					spac.ruleTable[key] = append(regs, reg)
				}
			}
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
	for name, rules := range spac.ruleTable {
		for _, rule := range rules {
			if rule.MatchString(url) {
				proxyName = name
				break
			}
		}
	}
	v, ok := registedRemoteConnManager[proxyName]
	if !ok{
	  log.Printf("No proxy:%s defined.\n", proxyName)
	}
	return v, ok
}

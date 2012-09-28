package proxy

import (
	"common"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"
)

type JsonRule struct {
	Method       []string
	Host         []string
	URL          []string
	Proxy        string
	Attr         map[string]string
	method_regex []*regexp.Regexp
	host_regex   []*regexp.Regexp
	url_regex    []*regexp.Regexp
}

type SpacRule struct {
}

func matchRegexs(str string, rules []*regexp.Regexp) bool {
	//log.Printf("Match regex:%d for str:%s\n", len(rules), str)
	if len(rules) == 0 {
		return true
	}
	for _, regex := range rules {
		if regex.MatchString(str) {
			//log.Printf("Success to match regex:%s for str:%s\n", regex.String(), str)
			return true
		}

	}
	return false
}

func initRegexSlice(rules []string) ([]*regexp.Regexp, error) {
	regexs := make([]*regexp.Regexp, 0)
	for _, originrule := range rules {
		rule := strings.TrimSpace(originrule)
		rule = strings.Replace(rule, ".", "\\.", -1)
		rule = strings.Replace(rule, "*", ".*", -1)

		reg, err := regexp.Compile(rule)
		if nil != err {
			log.Printf("Invalid pattern:%s for reason:%v\n", originrule, err)
			return nil, err
		} else {
			regexs = append(regexs, reg)
		}
	}

	return regexs, nil
}

func (r *JsonRule) init() (err error) {
	r.method_regex, err = initRegexSlice(r.Method)
	if nil != err {
		return
	}
	r.host_regex, err = initRegexSlice(r.Host)
	if nil != err {
		return
	}
	r.url_regex, err = initRegexSlice(r.URL)

	return
}

func (r *JsonRule) match(req *http.Request) bool {
	return matchRegexs(req.Method, r.method_regex) && matchRegexs(req.Host, r.host_regex) && matchRegexs(req.RequestURI, r.url_regex)
}

type SpacConfig struct {
	defaultRule string
	rules       []*JsonRule
}

var spac *SpacConfig

var registedRemoteConnManager map[string]RemoteConnectionManager = make(map[string]RemoteConnectionManager)

func RegisteRemoteConnManager(connManager RemoteConnectionManager) {
	registedRemoteConnManager[connManager.GetName()] = connManager
}

func InitSpac() {
	spac = &SpacConfig{}
	spac.defaultRule, _ = common.Cfg.GetProperty("SPAC", "Default")
	if len(spac.defaultRule) == 0 {
		spac.defaultRule = GAE_NAME
	}
	spac.rules = make([]*JsonRule, 0)
	if enable, exist := common.Cfg.GetIntProperty("SPAC", "Enable"); exist {
		if enable == 0 {
			return
		}
	}
	script, exist := common.Cfg.GetProperty("SPAC", "Script")
	if !exist{
	   script = "spac.json"
	}
	file, e := ioutil.ReadFile(common.Home + script)
	if e == nil {
		e = json.Unmarshal(file, &spac.rules)
		for _, json_rule := range spac.rules {
			e = json_rule.init()
		}
	}
	if nil != e {
		log.Printf("Failed to init SPAC for reason:%v", e)
	}
}

func SelectProxy(req *http.Request) (RemoteConnectionManager, bool) {
	proxyName := spac.defaultRule
	selected := false

	for _, r := range spac.rules {
		if r.match(req) {
			selected = true
			proxyName = r.Proxy
			break
		}
	}

	if selected {
		switch proxyName {
		case GAE_NAME, C4_NAME:
		case GOOGLE_NAME, GOOGLE_HTTP_NAME:
			return httpGoogleManager, true
		case GOOGLE_HTTPS_NAME:
			return httpsGoogleManager, true
		case DIRECT_NAME:
			forward := &Forward{overProxy: false}
			forward.target = req.Host
			if !strings.Contains(forward.target, ":") {
				forward.target = forward.target + ":80"
			}
			if !strings.Contains(forward.target, "://") {
				forward.target = "http://" + forward.target
			}
			return forward, true
		default:
			forward := &Forward{overProxy: true}
			forward.target = strings.TrimSpace(proxyName)
			if !strings.Contains(forward.target, "://") {
				forward.target = "http://" + forward.target
			}
			return forward, true
		}
	}

	v, ok := registedRemoteConnManager[proxyName]
	if !ok {
		log.Printf("No proxy:%s defined, use GAE instead.\n", proxyName)
		proxyName = GAE_NAME
		v, ok = registedRemoteConnManager[proxyName]
		if !ok {
			log.Printf("No GAE found.\n")
		}
	}
	return v, ok
}

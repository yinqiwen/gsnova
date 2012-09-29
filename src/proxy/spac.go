package proxy

import (
	"common"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type JsonRule struct {
	Method       []string
	Host         []string
	URL          []string
	Proxy        []string
	Attr         map[string]string
	method_regex []*regexp.Regexp
	host_regex   []*regexp.Regexp
	url_regex    []*regexp.Regexp
}

func matchRegexs(str string, rules []*regexp.Regexp) bool {
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

func decodeBase64PAC(content []byte) []byte {
	strcontent := string(content)
	//log.Printf("1##%s\n", strcontent)
	ss := strings.Split(strcontent, "eval(decode64(\"")
	//log.Printf("2##%d\n", len(ss))
	if len(ss) == 2 {
		ss = strings.Split(ss[1], "\"))")
		if len(ss) >= 1 {
			//log.Printf("%s\n", ss[0])
			content, _ = base64.StdEncoding.DecodeString(ss[0])
		}
	}
	return content
}

func updateAutoProxy2PAC(url string) {
	log.Printf("Fetch AutoProxy2PAC from %s\n", url)
	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		http_proxy := os.Getenv("http_proxy")
		https_proxy := os.Getenv("https_proxy")
		if addr, exist := common.Cfg.GetProperty("LocalServer", "Listen"); exist {
			_, port, _ := net.SplitHostPort(addr)
			os.Setenv("http_proxy", "http://"+net.JoinHostPort("127.0.0.1", port))
			os.Setenv("https_proxy", "http://"+net.JoinHostPort("127.0.0.1", port))
		}

		defer func() {
			os.Setenv("http_proxy", http_proxy)
			os.Setenv("https_proxy", https_proxy)
		}()
		resp, err = http.DefaultClient.Get(url)
	}
	if err != nil || resp.StatusCode != 200 {
		log.Printf("Failed to fetch AutoProxy2PAC from %s for reason:%v  %v\n", url, err)
	} else {
		body, err := ioutil.ReadAll(resp.Body)
		if nil == err {
			hf := common.Home + "/snova-gfwlist.pac"
			ioutil.WriteFile(hf, decodeBase64PAC(body), 0755)
		}
	}
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
	if url, exist := common.Cfg.GetProperty("SPAC", "AutoProxy2PAC"); exist {
		go updateAutoProxy2PAC(url)
	}

	script, exist := common.Cfg.GetProperty("SPAC", "Script")
	if !exist {
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

func SelectProxy(req *http.Request) []RemoteConnectionManager {
	proxyNames := []string{spac.defaultRule}
	proxyManagers := make([]RemoteConnectionManager, 0)

	matched := false
	for _, r := range spac.rules {
		if r.match(req) {
			proxyNames = r.Proxy
			matched = true
			break
		}
	}

	if !matched && hostsEnable {
		if _, exist := getOnlineMappingHost(req.Host); exist {
			proxyNames = []string{"Direct", spac.defaultRule}
		}
	}

	for _, proxyName := range proxyNames {
		switch proxyName {
		case GAE_NAME, C4_NAME:
			v, ok := registedRemoteConnManager[proxyName]
			if !ok {
				log.Printf("No proxy:%s defined, use GAE instead.\n", proxyName)
				proxyName = GAE_NAME
				v, ok = registedRemoteConnManager[proxyName]
				if !ok {
					log.Printf("No GAE found.\n")
				} else {
					proxyManagers = append(proxyManagers, v)
				}
			}
		case GOOGLE_NAME, GOOGLE_HTTP_NAME:
			proxyManagers = append(proxyManagers, httpGoogleManager)
		case GOOGLE_HTTPS_NAME:
			proxyManagers = append(proxyManagers, httpsGoogleManager)
		case DIRECT_NAME:
			forward := &Forward{overProxy: false}
			forward.target = req.Host
			if !strings.Contains(forward.target, ":") {
				forward.target = forward.target + ":80"
			}
			if !strings.Contains(forward.target, "://") {
				forward.target = "http://" + forward.target
			}
			proxyManagers = append(proxyManagers, forward)
		default:
			forward := &Forward{overProxy: true}
			forward.target = strings.TrimSpace(proxyName)
			if !strings.Contains(forward.target, "://") {
				forward.target = "http://" + forward.target
			}
			proxyManagers = append(proxyManagers, forward)
		}
	}

	return proxyManagers
}

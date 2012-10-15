package gfwlist

import (
	"bufio"
	"encoding/base64"
	"log"
	"net/http"
	"regexp"
	"strings"
	"util"
)

type gfwListRule interface {
	init(rule string) error
	match(req *http.Request) bool
}

type hostUrlWildcardRule struct {
	only_http bool
	host_rule string
	url_rule  string
}

func (r *hostUrlWildcardRule) init(rule string) (err error) {
	if !strings.Contains(rule, "/") {
		r.host_rule = rule
		return
	}
	rules := strings.SplitN(rule, "/", 2)
	r.host_rule = rules[0]
	if nil != err {
		return
	}
	if len(rules) == 2 && len(rules[1]) > 0 {
		r.url_rule = rules[1]
	}
	return
}

func (r *hostUrlWildcardRule) match(req *http.Request) bool {
	if r.only_http && strings.EqualFold(req.Method, "Connect") {
		return false
	}
	if ret := util.WildcardMatch(req.Host, r.host_rule); !ret {
		return false
	}
	if len(r.url_rule) > 0 {
		return util.WildcardMatch(req.URL.RequestURI(), r.url_rule)
	}
	return true
}

type urlWildcardRule struct {
	url_rule  string
}

func (r *urlWildcardRule) init(rule string) (err error) {
	r.url_rule = rule
	return
}

func (r *urlWildcardRule) match(req *http.Request) bool {
	return util.WildcardMatch(util.GetURLString(req, false), r.url_rule)
}

type urlRegexRule struct {
	is_raw_regex bool
	url_reg      *regexp.Regexp
}

func (r *urlRegexRule) init(rule string) (err error) {
	if r.is_raw_regex {
		r.url_reg, err = regexp.Compile(rule)
	} else {
		r.url_reg, err = util.PrepareRegexp(rule)
	}
	return
}

func (r *urlRegexRule) match(req *http.Request) bool {
	ret := r.url_reg.MatchString(util.GetURLString(req, false))
	//	if ret{
	//	   log.Printf("url is %s, rule is %s\n", util.GetURLString(req, false), r.url_reg.String())
	//	}
	return ret
}

type GFWList struct {
	white_list []gfwListRule
	black_list []gfwListRule
}

func (gfw *GFWList) IsBlockedByGFW(req *http.Request) bool {
	for _, rule := range gfw.white_list {
		if rule.match(req) {
			return false
		}
	}
	for _, rule := range gfw.black_list {
		if rule.match(req) {
			//log.Printf("matched for :%s for %v\n", req.URL.String(), rule)
			return true
		}
	}
	return false
}

func Parse(rules string) (*GFWList, error) {
	reader := bufio.NewReader(strings.NewReader(rules))
	gfw := new(GFWList)
	//i := 0
	for {
		line, _, err := reader.ReadLine()
		if nil != err {
			break
		}
		str := strings.TrimSpace(string(line))
		//comment
		if strings.HasPrefix(str, "!") || len(str) == 0 {
			continue
		}
		if strings.HasPrefix(str, "@@||") {
			rule := new(hostUrlWildcardRule)
			err := rule.init(str[4:])
			if nil != err {
				log.Printf("Failed to init exclude rule:%s for %v\n", str[4:], err)
				continue
			}
			gfw.white_list = append(gfw.white_list, rule)
		} else if strings.HasPrefix(str, "||") {
			rule := new(hostUrlWildcardRule)
			err := rule.init(str[2:])
			if nil != err {
				log.Printf("Failed to init host url rule:%s for %v\n", str[2:], err)
				continue
			}
			gfw.black_list = append(gfw.black_list, rule)
		} else if strings.HasPrefix(str, "|http") {
			rule := new(urlWildcardRule)
			err := rule.init(str[1:])
			if nil != err {
				log.Printf("Failed to init url rule:%s for %v\n", str[1:], err)
				continue
			}
			gfw.black_list = append(gfw.black_list, rule)
		} else if strings.HasPrefix(str, "/") && strings.HasSuffix(str, "/") {
			rule := new(urlRegexRule)
			rule.is_raw_regex = true
			err := rule.init(str[1 : len(str)-1])
			if nil != err {
				log.Printf("Failed to init url rule:%s for %v\n", str[1:len(str)-1], err)
				continue
			}
			gfw.black_list = append(gfw.black_list, rule)
		} else {
			rule := new(hostUrlWildcardRule)
			rule.only_http = true
			err := rule.init(str)
			if nil != err {
				log.Printf("Failed to init host url rule:%s for %v\n", str, err)
				continue
			}
			gfw.black_list = append(gfw.black_list, rule)
		}
	}
	return gfw, nil
}

func ParseRaw(rules string) (*GFWList, error) {
	content, err := base64.StdEncoding.DecodeString(string(rules))
	if err != nil {
		return nil, err
	}
	return Parse(string(content))
}

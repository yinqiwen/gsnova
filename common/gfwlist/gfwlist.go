package gfwlist

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/helper"
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
	if ret := helper.WildcardMatch(req.Host, r.host_rule); !ret {
		return false
	}
	if len(r.url_rule) > 0 {
		return helper.WildcardMatch(req.URL.RequestURI(), r.url_rule)
	}
	return true
}

type urlWildcardRule struct {
	url_rule string
}

func (r *urlWildcardRule) init(rule string) (err error) {
	if !strings.Contains(rule, "*") {
		rule = "*" + rule
	}
	r.url_rule = rule
	return
}

func (r *urlWildcardRule) match(req *http.Request) bool {
	return helper.WildcardMatch(helper.GetRequestURLString(req), r.url_rule)
}

type urlRegexRule struct {
	is_raw_regex bool
	url_reg      *regexp.Regexp
}

func (r *urlRegexRule) init(rule string) (err error) {
	if r.is_raw_regex {
		r.url_reg, err = regexp.Compile(rule)
	} else {
		r.url_reg, err = helper.PrepareRegexp(rule, false)
	}
	return
}

func (r *urlRegexRule) match(req *http.Request) bool {
	ret := r.url_reg.MatchString(helper.GetRequestURLString(req))
	//	if ret{
	//	   log.Printf("url is %s, rule is %s\n", util.GetURLString(req, false), r.url_reg.String())
	//	}
	return ret
}

type GFWList struct {
	white_list []gfwListRule
	black_list []gfwListRule
	mutex      sync.Mutex
}

func (gfw *GFWList) IsBlockedByGFW(req *http.Request) bool {
	gfw.mutex.Lock()
	defer gfw.mutex.Unlock()
	for _, rule := range gfw.white_list {
		if rule.match(req) {
			return false
		}
	}
	for _, rule := range gfw.black_list {
		if rule.match(req) {
			//log.Printf("matched for :%v for %v\n", req.URL, rule)
			return true
		}
	}
	return false
}

func (gfw *GFWList) clone(l *GFWList) {
	if nil == l {
		return
	}
	gfw.mutex.Lock()
	defer gfw.mutex.Unlock()
	gfw.white_list = l.white_list
	gfw.black_list = l.black_list
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
			//rule.only_http = true
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
	ioutil.WriteFile("./tmp.txt", content, 0666)
	return Parse(string(content))
}

func NewGFWList(u string, proxy string, watch bool) (*GFWList, error) {
	hc := &http.Client{}
	if len(proxy) > 0 {
		hc.Transport = &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				return url.Parse(proxy)
			},
		}
	}

	fetch := func(last time.Time) ([]byte, time.Time, error) {
		zero := time.Time{}
		resp, err := hc.Get(u)
		if nil != err {
			return nil, zero, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, zero, fmt.Errorf("Invalid response:%v", resp)
		}
		// lastModDate := resp.Header.Get("last-modified")
		// if len(lastModDate) == 0 {
		// 	return nil, zero, fmt.Errorf("No 'last-modified' header in head %v", resp.Header)
		// }
		// t, err := time.Parse(time.RFC1123, lastModDate)
		// if nil != err {
		// 	return nil, zero, err
		// }
		// if !t.After(last) {
		// 	return nil, t, fmt.Errorf("No updated content")
		// }
		t := time.Now()
		body, err := ioutil.ReadAll(resp.Body)
		if nil != err {
			return nil, zero, err
		}
		return body, t, nil
	}
	start := time.Time{}
	content, modTime, err := fetch(start)
	if nil != err {
		return nil, err
	}
	gfwlist, err := ParseRaw(string(content))
	if nil != err {
		return nil, err
	}
	if watch {
		go func() {
			for {
				select {
				case <-time.After(10 * time.Minute):
					newContent, newModTime, nerr := fetch(modTime)
					if nerr == nil {
						modTime = newModTime
						nlist, _ := ParseRaw(string(newContent))
						gfwlist.clone(nlist)
					}
				}
			}
		}()
	}
	return gfwlist, nil
}

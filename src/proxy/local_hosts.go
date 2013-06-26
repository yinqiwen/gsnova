package proxy

import (
	"common"
	"io/ioutil"
	"log"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"util"
)

type regexHost struct {
	regex    *regexp.Regexp
	selector *util.ListSelector
}

var mapping map[string]*util.ListSelector = make(map[string]*util.ListSelector)
var regexMappingArray = make([]*regexHost, 0)

func loadLocalHostMapping(file string) error {
	ini, err := util.LoadIniFile(file)
	if nil != err {
		return err
	}
	props, exist := ini.GetTagProperties("")
	if exist {
		for k, v := range props {
			selector := &util.ListSelector{}
			hs := strings.Split(v, "|")
			for _, h := range hs {
				selector.Add(h)
			}
			if strings.Contains(k, "*") {
				rh := new(regexHost)
				var err error
				if rh.regex, err = util.PrepareRegexp(k, true); nil != err {
					log.Printf("[ERROR]Invalid regex host pattern:%s for reason:%v\n", k, err)
					continue
				}
				rh.selector = selector
				regexMappingArray = append(regexMappingArray, rh)
			} else {
				mapping[k] = selector
			}
		}
	}
	return nil
}

func fetchCloudHosts(url string) {
	time.Sleep(5 * time.Second)
	log.Printf("Fetch remote clound hosts:%s\n", url)
	file := common.Home + "hosts/" + CLOUD_HOSTS_FILE
	var file_ts time.Time
	if fi, err := os.Stat(file); nil == err {
		file_ts = fi.ModTime()
	}
	body, _, err := util.FetchLateastContent(url, common.ProxyPort, file_ts, true)
	if nil == err && len(body) > 0 {
		ioutil.WriteFile(file, body, 0666)
		mapping = make(map[string]*util.ListSelector)
		loadLocalHostMappings()
	}
	if nil != err {
		log.Printf("Failed to fetch spac cloud hosts for reason:%v\n", err)
	}
}

func loadLocalHostMappings() error {
	loadLocalHostMapping(common.Home + "hosts/" + CLOUD_HOSTS_FILE)
	//user hosts has higher priority
	loadLocalHostMapping(common.Home + "hosts/" + USER_HOSTS_FILE)
	return nil
}

func isEntry(k string) bool {
	_, exist := mapping[k]
	return exist
}

func getLocalHostMapping(host string) (string, bool) {
	selectHost := func(s *util.ListSelector) (string, bool) {
		t := s.Select().(string)
		if isEntry(t) {
			t, _ = getLocalHostMapping(t)
			return t, true
		}
		return t, true
	}
	h, exist := mapping[host]
	if exist {
		return selectHost(h)
	}
	for _, v := range regexMappingArray {
		if v.regex.MatchString(host) {
			return selectHost(v.selector)
		}
	}
	return host, false
}

func getAddressMapping(addr string) string {
	v := strings.Split(addr, ":")
	if len(v) == 2 {
		tmp, _ := getLocalHostMapping(v[0])
		addr, _ = lookupAvailableAddress(net.JoinHostPort(tmp, v[1]), true)
		return addr
	}
	return addr
}

func getLocalUrlMapping(addr string) string {
	l, err := url.Parse(addr)
	if nil != err {
		return addr
	}
	v := strings.Split(l.Host, ":")
	if len(v) == 2 {
		tmp, _ := getLocalHostMapping(v[0])
		l.Host = tmp + ":" + v[1]
	} else {
		l.Host, _ = getLocalHostMapping(l.Host)
		if l.Scheme == "https" {
			l.Host = net.JoinHostPort(l.Host, "443")
		} else {
			l.Host = net.JoinHostPort(l.Host, "80")
		}
		l.Host, _ = lookupAvailableAddress(l.Host, true)
	}

	return l.String()
}

package proxy

import (
	"common"
	"io/ioutil"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
	"util"
)

var mapping map[string]*util.ListSelector = make(map[string]*util.ListSelector)

func loadLocalHostMapping(file string) error {
	ini, err := util.LoadIniFile(file)
	if nil != err {
		return err
	}
	props, exist := ini.GetTagProperties("")
	if exist {
		for k, v := range props {
			mapping[k] = &util.ListSelector{}
			hs := strings.Split(v, "|")
			for _, h := range hs {
				mapping[k].Add(h)
			}
		}
	}
	return nil
}

func fetchCloudHosts(url string) {
	time.Sleep(5 * time.Second)
	log.Printf("Fetch remote clound spac rule:%s\n", url)
	file := common.Home + "hosts/" + CLOUD_HOSTS_FILE
	var file_ts time.Time
	if fi, err := os.Stat(file); nil == err {
		file_ts = fi.ModTime()
	}
	body, _, err := util.FetchLateastContent(url, common.ProxyPort, file_ts, false)
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
	h, exist := mapping[host]
	if exist {
		t := h.Select().(string)
		if isEntry(t) {
			t, _ = getLocalHostMapping(t)
			return t, true
		} else {
			return t, true
		}
	}
	return host, false
}

//func getLocalHostPortMapping(addr string) (string) {
//	l, err := url.Parse(addr)
//	if nil != err {
//		return addr, "80"
//	}
//	v := strings.Split(l.Host, ":")
//	if len(v) == 2 {
//		return v[0], v[1]
//	}
//	h, _ := getLocalHostMapping(l.Host)
//	if strings.EqualFold(l.Scheme, "https") {
//		return net.JoinHostPort(h, "443")
//	}
//	return net.JoinHostPort(h, "80")
//}

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
        //l.Host,_ = lookupAvailableAddress(l.Host)
	}
	
	return l.String()
}

package util

import (
	//"fmt"
	"net/url"
	"strings"
)

var mapping map[string]*ListSelector = make(map[string]*ListSelector)

func LoadHostMapping(file string) error {
	ini, err := LoadIniFile(file)
	if nil != err {
		return err
	}
	props, exist := ini.GetTagProperties("")
	if exist {
		for k, v := range props {
			mapping[k] = &ListSelector{}
			hs := strings.Split(v, "|")
			for _, h := range hs {
				mapping[k].Add(h)

			}
		}
	}
	return nil
}

func isEntry(k string) bool {
	_, exist := mapping[k]
	return exist
}

func GetHost(host string) string {
	h, exist := mapping[host]
	if exist {
		t := h.Select().(string)
		if isEntry(t) {
			return GetHost(t)
		} else {
			return t
		}
	}
	return host
}

func GetUrl(addr string) string {
	//fmt.Printf("###%s\n", addr)
	l, err := url.Parse(addr)
	if nil != err {
		//fmt.Printf("$$$%s\n", err.Error())
		return addr
	}
	//fmt.Printf("2###%s\n", addr)
	v := strings.Split(l.Host, ":")
	if len(v) == 2 {
		l.Host = GetHost(v[0]) + ":" + v[1]
	} else {
		//fmt.Printf("3###%s\n", l.Host)
		l.Host = GetHost(l.Host)
	}
	return l.String()
}

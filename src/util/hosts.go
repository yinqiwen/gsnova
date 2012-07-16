package util

import (
	"net/url"
	"strings"
)

var mapping map[string]ListSelector = make(map[string]ListSelector)

func LoadHostMapping(file string) error {
    
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
		}
	}
	return host
}

func GetUrl(addr string) string {
	l, err := url.Parse(addr)
	if nil != err {
		return addr
	}
	v := strings.Split(l.Host, ":")
	if len(v) == 2 {
		l.Host = GetHost(v[0]) + ":" + v[1]
	} else {
		l.Host = GetHost(l.Host)
	}
	return l.String()
}

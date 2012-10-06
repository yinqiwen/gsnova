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

func GetHostMapping(host string) (string, bool) {
	h, exist := mapping[host]
	if exist {
		t := h.Select().(string)
		if isEntry(t) {
			t, _ = GetHostMapping(t)
			return t, true
		} else {
			return t, true
		}
	}
	return host, false
}

//func GetHost(host string) string {
//	h, exist := mapping[host]
//	if exist {
//		t := h.Select().(string)
//		if isEntry(t) {
//			return GetHost(t)
//		} else {
//			return t
//		}
//	}
//	return host
//}

func GetHostPort(addr string) (string, string) {
	//fmt.Printf("###%s\n", addr)
	l, err := url.Parse(addr)
	if nil != err {
		return addr, "80"
	}
	//fmt.Printf("2###%s\n", addr)
	v := strings.Split(l.Host, ":")
	if len(v) == 2 {
		return v[0], v[1]
	}
	h, _ := GetHostMapping(l.Host)
	if strings.EqualFold(l.Scheme, "https") {
		return h, "443"
	}
	return h, "80"
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
		tmp, _ := GetHostMapping(v[0])
		l.Host = tmp + ":" + v[1]
	} else {
		//fmt.Printf("3###%s\n", l.Host)
		l.Host, _ = GetHostMapping(l.Host)
	}
	return l.String()
}

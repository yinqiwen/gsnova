package hosts

import (
	"encoding/json"
	"net"
	"regexp"
	"strings"
	"sync"

	"github.com/yinqiwen/gsnova/common/helper"
)

const SNIProxy = "sni_proxy"

type hostMapping struct {
	host      string
	hostRegex *regexp.Regexp
	mapping   []string
	cursor    int
}

func (h *hostMapping) Get() string {
	c := h.cursor
	if c >= len(h.mapping) {
		c = 0
	}
	s := h.mapping[c]
	h.cursor = c + 1
	return s
}

var hostMappingTable = make(map[string]*hostMapping)
var mappingMutex sync.Mutex

func getHost(host string) (string, bool) {
	var ok bool
	mapping, exist := hostMappingTable[host]
	if exist {
		s := mapping.Get()
		ok := true
		if !strings.Contains(s, ".") { //alials name
			s, ok = getHost(s)
		}
		return s, ok
	}
	for _, m := range hostMappingTable {
		if nil != m.hostRegex {
			if m.hostRegex.MatchString(host) {
				s := m.Get()
				if !strings.Contains(s, ".") { //alials name
					s, ok = getHost(s)
				} else {
					hostMappingTable[host] = m
					ok = true
				}
				return s, ok
			}
		}
	}
	return host, false
}

func GetHost(host string) string {
	mappingMutex.Lock()
	defer mappingMutex.Unlock()
	s, _ := getHost(host)
	return s
}

func GetAddr(addr string, defaultPort string) string {
	host, port, err := net.SplitHostPort(addr)
	if nil != err {
		host = addr
	}
	if net.ParseIP(host) == nil {
		mappingMutex.Lock()
		defer mappingMutex.Unlock()
		host, _ = getHost(host)
	}
	if nil == err {
		return net.JoinHostPort(host, port)
	}
	return net.JoinHostPort(host, defaultPort)
}

func InHosts(host string) bool {
	if strings.Contains(host, ":") {
		return true
	}
	mappingMutex.Lock()
	defer mappingMutex.Unlock()
	if strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}
	_, ok := getHost(host)
	return ok
}

func Clear() {
	mappingMutex.Lock()
	defer mappingMutex.Unlock()
	hostMappingTable = make(map[string]*hostMapping)
}

func Init(confile string) error {
	//file := "hosts.json"
	hs := make(map[string][]string)
	data, err := helper.ReadWithoutComment(confile, "//")
	if nil == err {
		err = json.Unmarshal(data, &hs)
	}
	if nil != err {
		//fmt.Printf("Failed to load hosts config:%s for reason:%v", string(data), err)
		return err
	}
	mappingMutex.Lock()
	defer mappingMutex.Unlock()
	hostMappingTable = make(map[string]*hostMapping)
	for k, vs := range hs {
		if len(vs) > 0 {
			mapping := new(hostMapping)
			mapping.host = k
			mapping.mapping = vs
			if strings.Contains(k, "*") {
				rule := strings.Replace(k, "*", ".*", -1)
				mapping.hostRegex, _ = regexp.Compile(rule)
			}
			hostMappingTable[k] = mapping
		}
	}
	return nil
}

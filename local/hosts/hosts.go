package hosts

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"
)

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
	h.cursor++
	return s
}

var hostMappingTable = make(map[string]*hostMapping)
var mappingMutex sync.Mutex

func GetHost(host string) string {
	mappingMutex.Lock()
	defer mappingMutex.Unlock()
	mapping, exist := hostMappingTable[host]
	if exist {
		return mapping.Get()
	}
	for _, m := range hostMappingTable {
		if nil != m.hostRegex {
			if m.hostRegex.MatchString(host) {
				hostMappingTable[host] = m
				s := m.Get()
				if !strings.Contains(s, ".") { //alials name
					return GetHost(s)
				} else {
					return s
				}
			}
		}
	}
	return host
}

func init() {
	file := "hosts.json"
	hs := make(map[string][]string)
	data, err := ioutil.ReadFile(file)
	if nil == err {
		err = json.Unmarshal(data, &hs)
	}
	if nil != err {
		fmt.Printf("Failed to load hosts config:%s for reason:%v", file, err)
		return
	}
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
}

package gfwlist

import (
	"bufio"
	"encoding/base64"
	"net/http"
	"strings"
)

type gfwListRule interface {
	match(req *http.Request) bool
}

type GFWList struct {
	exclude []gfwListRule
	match   []gfwListRule
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
		} else if strings.HasPrefix(str, "||") {
		} else if strings.HasPrefix(str, "|http") {
		} else if strings.HasPrefix(str, "/") && strings.HasSuffix(str, "/") {
		} else {
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

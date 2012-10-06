package proxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
)

func redirectHttps(conn net.Conn, req *http.Request) {
	conn.Write([]byte("HTTP/1.1 302 Found\r\n"))
	location := fmt.Sprintf("Location:https://%s%s\r\nConnection:close\r\n\r\n", req.Host, req.RequestURI)
	if strings.HasPrefix(req.RequestURI, "http://") {
		location = fmt.Sprintf("Location:%s\r\nConnection:close\r\n\r\n", strings.Replace(req.RequestURI, "http", "https", 1))
	}
	conn.Write([]byte(location))
	conn.Close()
}

func initHostMatchRegex(pattern string) []*regexp.Regexp {
	regexs := []*regexp.Regexp{}
	ps := strings.Split(strings.TrimSpace(pattern), "|")
	for _, p := range ps {
		originrule := p
		p = strings.TrimSpace(p)
		p = strings.Replace(p, ".", "\\.", -1)
		p = strings.Replace(p, "*", ".*", -1)
		reg, err := regexp.Compile(p)
		if nil != err {
			log.Printf("[ERROR]Invalid pattern:%s for reason:%v\n", originrule, err)
		} else {
			regexs = append(regexs, reg)
		}
	}
	return regexs
}

func hostPatternMatched(patterns []*regexp.Regexp, host string) bool {
	for _, regex := range patterns {
		if regex.MatchString(host) {
			return true
		}
	}
	return false
}

type rangeChunk struct {
	start   int
	content []byte
}

type rangeReader struct {
	start   int
	length int
	reader  io.ReadCloser
}

package proxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"util"
)

func containsAttr(attrs map[string]string, key string) bool{
   if nil == attrs{
      return false
   }
   _, exist := attrs[key]
   return exist
}

func redirectHttps(conn net.Conn, req *http.Request) {
	conn.Write([]byte("HTTP/1.1 302 Found\r\n"))
	location := fmt.Sprintf("Location:https://%s%s\r\nConnection:close\r\n\r\n", req.Host, req.RequestURI)
	if strings.HasPrefix(req.RequestURI, "http://") {
		location = fmt.Sprintf("Location:%s\r\nConnection:close\r\n\r\n", strings.Replace(req.RequestURI, "http", "https", 1))
	}
	//log.Printf("redirectHttps %s\n", req.Host)
	conn.Write([]byte(location))
	conn.Close()
}

func initHostMatchRegex(pattern string) []*regexp.Regexp {
	regexs := []*regexp.Regexp{}
	ps := strings.Split(strings.TrimSpace(pattern), "|")
	for _, p := range ps {
		reg, err := util.PrepareRegexp(p, true)
		if nil != err {
			log.Printf("[ERROR]Invalid pattern:%s for reason:%v\n", p, err)
		} else {
			regexs = append(regexs, reg)
		}
	}
	return regexs
}

func hostPatternMatched(patterns []*regexp.Regexp, host string) bool {
	for _, regex := range patterns {
		if regex.MatchString(host) {
		    //log.Printf("#######Pattern is %s for host %s\n", regex.String(), host)
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

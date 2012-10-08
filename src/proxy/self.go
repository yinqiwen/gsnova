package proxy

import (
	"bytes"
	"common"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"runtime"
	"strings"
)

//type selfResponseWriter struct {
//	header http.Header
//	conn   net.Conn
//	wroteHeader bool
//}
//
//func (w *selfResponseWriter) Header() http.Header {
//	if nil == w.header {
//		w.header = make(http.Header)
//	}
//	return w.header
//}
//func (w *selfResponseWriter) Write([]byte) (int, error) {
//    
//}
//
//func (w *selfResponseWriter) WriteHeader(int) {
//}
//
//func InitSelfWebServer() {
//	http.DefaultServeMux.HandleFunc("/", statHandler)
//	http.DefaultServeMux.HandleFunc("/stat", statHandler)
//	http.DefaultServeMux.HandleFunc("/gfwlist/pac", gfwlistPACHandler)
//}
//
//func statHandler(w http.ResponseWriter, req *http.Request) {
//	// http.
//}
//
//func gfwlistPACHandler(w ResponseWriter, req *Request) {
//	http.ServeFile(w, r, common.Home+"/snova-gfwlist.pac")
//}

func dummyReq(method string) *http.Request {
	return &http.Request{Method: method}
}

func statHandler(req *http.Request) *http.Response {
	res := &http.Response{Status: "200 OK",
		StatusCode: 200,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Request:    dummyReq("GET"),
		Header: http.Header{
			"Connection":   {"close"},
			"Content-Type": {"text/plain"},
		},
		Close:         true,
		ContentLength: -1}
	var stat runtime.MemStats
	runtime.ReadMemStats(&stat)
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("HostPortBlockVerifyResultSize: %d\n", len(blockVerifyResult)))
	buf.WriteString(fmt.Sprintf("HostMappingSize: %d\n", len(hostMapping)))
	buf.WriteString(fmt.Sprintf("ReachableDNSResultSize: %d\n", len(reachableDNSResult)))
	if content, err := json.MarshalIndent(&stat, "", " "); nil == err {
		buf.Write(content)
	}
	res.Body = ioutil.NopCloser(&buf)
	return res
}

func indexHandler(req *http.Request) *http.Response {
	res := &http.Response{Status: "200 OK",
		StatusCode: 200,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Request:    dummyReq("GET"),
		Header: http.Header{
			"Connection":   {"close"},
			"Content-Type": {"text/html"},
		},
		Close:         true,
		ContentLength: -1}
	hf := common.Home + "/web/html/index.html"
	if content, err := ioutil.ReadFile(hf); nil == err {
		strcontent := string(content)
		strcontent = strings.Replace(strcontent, "${Version}", common.Version, -1)
		strcontent = strings.Replace(strcontent, "${ProxyPort}", common.ProxyPort, -1)
		var buf bytes.Buffer
		buf.WriteString(strcontent)
		res.Body = ioutil.NopCloser(&buf)
	}
	return res
}

func pacHandler(req *http.Request) *http.Response {
	res := &http.Response{Status: "200 OK",
		StatusCode: 200,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Request:    dummyReq("GET"),
		Header: http.Header{
			"Connection":          {"close"},
			"Content-Type":        {"application/x-ns-proxy-autoconfig"},
			"Content-Disposition": {"attachment;filename=snova-gfwlist.pac"},
		},
		Close:         true,
		ContentLength: -1}
	hf := common.Home + "/snova-gfwlist.pac"
	if content, err := ioutil.ReadFile(hf); nil == err {
		var buf bytes.Buffer
		buf.Write(content)
		res.Body = ioutil.NopCloser(&buf)
	}
	return res
}

func handleSelfHttpRequest(req *http.Request, conn net.Conn) {
	path := req.URL.Path
	log.Printf("Path is %s\n", path)
	var res *http.Response
	switch path {
	case "/pac/gfwlist":
		res = pacHandler(req)
	case "/stat":
		res = statHandler(req)
	case "/":
		res = indexHandler(req)
	}
	if nil != res {
		res.Write(conn)
	}
	conn.Close()
}

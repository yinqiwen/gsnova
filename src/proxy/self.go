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
	"util"
)

var lp *util.DelegateConnListener

func InitSelfWebServer() {
	lp = util.NewDelegateConnListener()
	http.HandleFunc("/pac/gfwlist", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/snova-gfwlist.pac"
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Header().Set("Content-Disposition", "attachment;filename=snova-gfwlist.pac")
		http.FileServer(http.Dir(common.Home)).ServeHTTP(w, r)
	})
	http.HandleFunc("/stat", statHandler)
	http.HandleFunc("/", indexHandler)
	http.Handle("/*", http.NotFoundHandler())
	go http.Serve(lp, nil)
}

func indexHandler(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/" {
		http.NotFoundHandler().ServeHTTP(w, req)
		return
	}
	hf := common.Home + "/web/html/index.html"
	if content, err := ioutil.ReadFile(hf); nil == err {
		strcontent := string(content)
		strcontent = strings.Replace(strcontent, "${Version}", common.Version, -1)
		strcontent = strings.Replace(strcontent, "${ProxyPort}", common.ProxyPort, -1)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(strcontent))
	}
}

//
func statHandler(w http.ResponseWriter, req *http.Request) {
	var stat runtime.MemStats
	runtime.ReadMemStats(&stat)
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("HostPortBlockVerifyResultSize: %d\n", len(blockVerifyResult)))
	buf.WriteString(fmt.Sprintf("HostMappingSize: %d\n", len(hostMapping)))
	buf.WriteString(fmt.Sprintf("ReachableDNSResultSize: %d\n", len(reachableDNSResult)))
	if content, err := json.MarshalIndent(&stat, "", " "); nil == err {
		buf.Write(content)
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(buf.Bytes())
}

func handleSelfHttpRequest(req *http.Request, conn net.Conn) {
	log.Printf("Path is %s\n", req.URL.Path)
	lp.Delegate(conn, req)
}

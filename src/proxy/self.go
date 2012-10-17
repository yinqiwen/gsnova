package proxy

import (
	"bytes"
	"common"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"runtime"
	"util"
)

var lp *util.DelegateConnListener

func InitSelfWebServer() {
	lp = util.NewDelegateConnListener()
	http.Handle("/css/", http.FileServer(http.Dir(common.Home+"/css")))
	http.Handle("/js/", http.FileServer(http.Dir(common.Home+"/js")))
	http.HandleFunc("/pac/gfwlist", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/spac/snova-gfwlist.pac"
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Header().Set("Content-Disposition", "attachment;filename=snova-gfwlist.pac")
		http.FileServer(http.Dir(common.Home)).ServeHTTP(w, r)
	})
	http.HandleFunc("/stat", statHandler)
	http.HandleFunc("/", indexHandler)
	go http.Serve(lp, nil)
}

func indexHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Connection", "close")
	if req.URL.Path != "/" {
		http.NotFound(w, req)
		return
	}
	hf := common.Home + "/web/html/index.html"
	if t, err := template.ParseFiles(hf); nil == err {
		type PageContent struct {
			Version   string
			ProxyPort string
		}
		t.Execute(w, &PageContent{common.Version, common.ProxyPort})
	}
}

func statHandler(w http.ResponseWriter, req *http.Request) {
	var stat runtime.MemStats
	runtime.ReadMemStats(&stat)
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("HostPortBlockVerifyResultSize: %d\n", len(blockVerifyResult)))
	buf.WriteString(fmt.Sprintf("HostMappingSize: %d\n", len(hostMapping)))
	buf.WriteString(fmt.Sprintf("ReachableDNSResultSize: %d\n", len(reachableDNSResult)))
	buf.WriteString(fmt.Sprintf("NumGoroutine: %d\n", runtime.NumGoroutine()))
	buf.WriteString(fmt.Sprintf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(runtime.NumCPU())))
	if content, err := json.MarshalIndent(&stat, "", " "); nil == err {
		buf.Write(content)
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Connection", "close")
	w.Write(buf.Bytes())
}

func handleSelfHttpRequest(req *http.Request, conn net.Conn) {
	lp.Delegate(conn, req)
}

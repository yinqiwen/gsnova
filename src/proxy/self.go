package proxy

import (
	"bytes"
	"common"
	"encoding/json"
	"event"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"util"
)

var lp *util.DelegateConnListener

func InitSelfWebServer() {
	lp = util.NewDelegateConnListener()
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		http.FileServer(http.Dir(common.Home+"/web")).ServeHTTP(w, r)
	})
	http.HandleFunc("/share.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		http.FileServer(http.Dir(common.Home+"/web")).ServeHTTP(w, r)
	})
	http.HandleFunc("/css/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		http.FileServer(http.Dir(common.Home+"/web")).ServeHTTP(w, r)
	})
	http.HandleFunc("/scripts/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		http.FileServer(http.Dir(common.Home+"/web")).ServeHTTP(w, r)
	})
	http.HandleFunc("/images/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		http.FileServer(http.Dir(common.Home+"/web")).ServeHTTP(w, r)
	})
	http.HandleFunc("/pac/gfwlist", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/spac/snova-gfwlist.pac"
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Header().Set("Content-Disposition", "attachment;filename=snova-gfwlist.pac")
		http.FileServer(http.Dir(common.Home)).ServeHTTP(w, r)
	})
	http.HandleFunc("/stat", statHandler)
	http.HandleFunc("/share", shareHandler)
	http.HandleFunc("/exit", exitHandler)
	http.HandleFunc("/", indexHandler)
	go http.Serve(lp, nil)
}

func indexHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Connection", "close")
	if req.URL.Path != "/" && !strings.HasSuffix(req.URL.Path, ".html") {
		log.Printf("Not found %v\n", req.URL)
		http.NotFound(w, req)
		return
	}

	dir := common.Home + "/web"
	path := "/index.html"
	if req.URL.Path != "/" {
		path = req.URL.Path
	}
	hf := dir + path
	if t, err := template.ParseFiles(hf); nil == err {
		type PageContent struct {
			Product   string
			Version   string
			ProxyPort string
		}
		t.Execute(w, &PageContent{common.Product, common.Version, common.ProxyPort})
	}
}

func exitHandler(w http.ResponseWriter, req *http.Request) {
	os.Exit(1)
}

func shareHandler(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	log.Printf("Request from is %v\n", req.Form)
	w.Header().Set("Connection", "close")
	if nil != singleton_gae {
		op := event.APPID_SHARE
		if len(req.Form.Get("share")) == 0 {
			op = event.APPID_UNSHARE
		}
		err := singleton_gae.shareAppId(req.Form.Get("appid"), req.Form.Get("email"), op)
		if nil == err {
			w.Write([]byte("Success!"))
		} else {
			w.Write([]byte(err.Error()))
		}
		return
	}
	w.WriteHeader(500)
}

func statHandler(w http.ResponseWriter, req *http.Request) {
	runtime.GC()
	var stat runtime.MemStats
	runtime.ReadMemStats(&stat)
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("HostMappingSize: %d\n", len(hostMapping)))
	buf.WriteString(fmt.Sprintf("BlockVerifyCacheSize: %d\n", len(blockVerifyCache)))
	buf.WriteString(fmt.Sprintf("NumGoroutine: %d\n", runtime.NumGoroutine()))
	buf.WriteString(fmt.Sprintf("NumProxyConn: %d\n", total_proxy_conn_num))
	buf.WriteString(fmt.Sprintf("NumGAEConn: %d\n", total_gae_conn_num))
	buf.WriteString(fmt.Sprintf("NumGoogleConn: %d\n", total_google_conn_num))
	buf.WriteString(fmt.Sprintf("NumForwardConn: %d\n", total_forwaed_conn_num))
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

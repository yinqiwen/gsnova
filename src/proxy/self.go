package proxy

import (
	"archive/zip"
	"bytes"
	"common"
	"event"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/rand"
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
	http.HandleFunc("/genrc4", rc4Handler)
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

func processC4ServerPackage(file string, key []byte) error {
	r, err := zip.OpenReader(common.Home + "/" + file)
	if err != nil {
		log.Printf("Failed to zip file for reason:%v", err)
		return err
	}
	w, err := os.Create(common.Home + "/" + string(key) + "_" + file)
	if err != nil {
		log.Printf("Failed to create zip file for reason:%v", err)
		return err
	}
	wr := zip.NewWriter(w)
	defer w.Close()
	defer r.Close()
	defer wr.Close()
	for _, f := range r.File {
		x, err := wr.Create(f.Name)
		if err != nil {
			return err
		}
		if strings.HasSuffix(f.Name, "rc4.key") {
			x.Write(key)
		} else {
			rr, err := f.Open()
			if err != nil {
				return err
			}
			io.Copy(x, rr)
			rr.Close()
		}
	}
	r.Close()
	wr.Close()
	return nil
}

func rc4Handler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Connection", "close")
	alphabet := "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	rc4key := make([]byte, 32)
	for i := 0; i < len(rc4key); i++ {
		rc4key[i] = alphabet[rand.Int()%len(alphabet)]
	}
	found_file := false
	f, err := os.Open(common.Home)
	if nil == err {
		fs, err := f.Readdir(-1)
		if nil == err {
			for _, file := range fs {
				if strings.HasSuffix(file.Name(), ".war") || strings.HasSuffix(file.Name(), ".zip") {
					ss := strings.Split(file.Name(), "_")
					if len(ss) == 2 && len(ss[0]) == len(rc4key) {
						continue
					}
					if err := processC4ServerPackage(file.Name(), rc4key); nil == err {
						found_file = true
					} else {
						log.Printf("Failed to process file:%s for reason:%v", file.Name(), err)
					}
				}
			}
		}
	}
	content := "<p>" + string(rc4key) + "</p><p>Please find processed server files in " + common.Home + "</p>"
	if !found_file {
		content = "<p>" + string(rc4key) + "</p><p>No valid C4 server package files in " + common.Home + "</p>"
	}
	w.Write([]byte(content))
}

func exitHandler(w http.ResponseWriter, req *http.Request) {
	os.Exit(1)
}

func shareHandler(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	log.Printf("Request form is %v\n", req.Form)
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
	//var stat runtime.MemStats
	//runtime.ReadMemStats(&stat)
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("HostMappingSize: %d\n", len(hostMapping)))
	buf.WriteString(fmt.Sprintf("BlockVerifyCacheSize: %d\n", len(blockVerifyCache)))
	buf.WriteString(fmt.Sprintf("NumGoroutine: %d\n", runtime.NumGoroutine()))
	buf.WriteString(fmt.Sprintf("NumProxyConn: %d\n", total_proxy_conn_num))
	buf.WriteString(fmt.Sprintf("NumGAEConn: %d\n", total_gae_conn_num))
	buf.WriteString(fmt.Sprintf("NumC4Conn: %d\n", total_c4_conn_num))
	buf.WriteString(fmt.Sprintf("NumC4Goroutine: %d\n", total_c4_routines))
	buf.WriteString(fmt.Sprintf("NumGoogleConn: %d\n", total_google_conn_num))
	buf.WriteString(fmt.Sprintf("NumGoogleGoroutine: %d\n", total_google_routine_num))
	buf.WriteString(fmt.Sprintf("NumForwardConn: %d\n", total_forwared_routine_num))
	buf.WriteString(fmt.Sprintf("NumForwardGoroutine: %d\n", total_forwared_routine_num))
	buf.WriteString(fmt.Sprintf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(runtime.NumCPU())))

	//	if content, err := json.MarshalIndent(&stat, "", " "); nil == err {
	//		buf.Write(content)
	//	}
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Connection", "close")
	w.Write(buf.Bytes())
}

func handleSelfHttpRequest(req *http.Request, conn net.Conn) {
	lp.Delegate(conn, req)
}

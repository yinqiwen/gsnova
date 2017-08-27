package proxy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	//_ "net/http/pprof"
	"os"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gotoolkit/ots"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/local"
)

func getConfigList(w http.ResponseWriter, r *http.Request) {
	var confs []string
	files, _ := ioutil.ReadDir(GConf.Admin.ConfigDir)
	for _, f := range files {
		if f.IsDir() {
			confs = append(confs, f.Name())
		}
	}
	w.Header().Set("Content-Type", "application/json")
	js, _ := json.Marshal(confs)
	w.Write(js)
}

func statCallback(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	fmt.Fprintf(w, "Version: %s\n", local.Version)
	//fmt.Fprintf(w, "NumSession: %d\n", getProxySessionSize())
	if nil != dnsCache {
		fmt.Fprintf(w, "DNSCacheSize: %d\n", dnsCache.Len())
	}
	ots.Handle("stat", w)
	// for _, p := range proxyTable {
	// 	p.PrintStat(w)
	// }
	//dumpProxySessions(w)
}
func stackdumpCallback(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(200)
	ots.Handle("stackdump", w)
}
func memdumpCallback(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(200)
	ots.Handle("MemProfile", w)
}
func gcCallback(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(200)
	ots.Handle("gc", w)
}

func startAdminServer() {
	if len(GConf.Admin.Listen) == 0 {
		return
	}
	// go func() {
	// 	log.Println(http.ListenAndServe("localhost:6060", nil))
	// }()
	if len(GConf.Admin.ConfigDir) == 0 {
		log.Printf("[WARN]The ConfigDir's Dir is empty, use current dir instead")
		GConf.Admin.ConfigDir = "./"
	}
	if len(GConf.Admin.BroadcastAddr) > 0 {
		ticker := time.NewTicker(3 * time.Second)
		localIP := helper.GetLocalIPv4()
		_, adminPort, _ := net.SplitHostPort(GConf.Admin.Listen)
		go func() {
			for _ = range ticker.C {
				addr, err := net.ResolveUDPAddr("udp", GConf.Admin.BroadcastAddr)
				var c *net.UDPConn
				if nil == err {
					c, err = net.DialUDP("udp", nil, addr)
				}
				if err != nil {
					log.Printf("Failed to resolve multicast addr.")
				} else {
					for _, ip := range localIP {
						c.Write([]byte(net.JoinHostPort(ip, adminPort)))
					}
				}
			}
		}()
	}
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir(GConf.Admin.ConfigDir))
	mux.Handle("/", fs)
	mux.HandleFunc("/_conflist", getConfigList)
	mux.HandleFunc("/stat", statCallback)
	mux.HandleFunc("/stackdump", stackdumpCallback)
	mux.HandleFunc("/gc", gcCallback)
	mux.HandleFunc("/memdump", memdumpCallback)
	err := http.ListenAndServe(GConf.Admin.Listen, mux)
	if nil != err {
		log.Printf("[ERROR]Failed to start config store server:%v", err)
	}

}

var syncClient *http.Client

func syncConfigFile(addr string, localDir, remoteDir string, fileName string) (bool, error) {
	filePath := remoteDir + "/" + fileName
	localFilePath := localDir + "/" + filePath
	localInfo, err := os.Stat(localFilePath)
	fileURL := "http://" + addr + "/" + filePath
	if err == nil {
		localFileModTime := localInfo.ModTime()
		headResp, err := syncClient.Head(fileURL)
		if nil != err {
			return false, err
		}
		lastModifiedHeader, _ := http.ParseTime(headResp.Header.Get("Last-Modified"))
		if lastModifiedHeader.Before(localFileModTime) {
			//log.Printf("Config file:%v is not update.", localFilePath)
			return false, nil
		}
	}
	resp, err := syncClient.Get(fileURL)
	if nil != err {
		return false, err
	}
	if resp.StatusCode != 200 || nil == resp.Body {
		if nil != resp.Body {
			resp.Body.Close()
		}
		return false, fmt.Errorf("Invalid response:%v for %s", resp, filePath)
	}

	data, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	os.Mkdir(localDir+"/"+remoteDir, 0775)
	ioutil.WriteFile(localFilePath, data, 0660)
	log.Printf("Persistent config:%s.", localDir+"/"+filePath)
	return true, nil
}

func SyncConfig(addr string, localDir string) (bool, error) {
	if nil == syncClient {
		tr := &http.Transport{}
		tr.ResponseHeaderTimeout = 15 * time.Second
		tr.Dial = netx.Dial
		syncClient = &http.Client{}
		syncClient.Timeout = 15 * time.Second
		syncClient.Transport = tr
	}
	resp, err := syncClient.Get("http://" + addr + "/_conflist")
	if nil != err {
		log.Printf("Error %v with local ip:%v", err, helper.GetLocalIPv4())
		return false, err
	}
	data, _ := ioutil.ReadAll(resp.Body)
	var confList []string
	json.Unmarshal(data, &confList)

	update := false
	for _, conf := range confList {
		u := false
		u, err = syncConfigFile(addr, localDir, conf, "client.json")
		if nil != err {
			return false, err
		}
		if u {
			update = true
		}
		u, err = syncConfigFile(addr, localDir, conf, "hosts.json")
		if nil != err {
			return false, err
		}
		if u {
			update = true
		}
		if u {
			log.Printf("Synced config:%s success", conf)
		}
	}
	return update, nil
}

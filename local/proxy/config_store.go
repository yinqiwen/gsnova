package proxy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/common/helper"
)

func getConfigList(w http.ResponseWriter, r *http.Request) {
	var confs []string
	files, _ := ioutil.ReadDir(GConf.ConfigStore.Dir)
	for _, f := range files {
		if f.IsDir() {
			confs = append(confs, f.Name())
		}
	}
	w.Header().Set("Content-Type", "application/json")
	js, _ := json.Marshal(confs)
	w.Write(js)
}

func startConfigStoreServer() {
	if len(GConf.ConfigStore.Listen) == 0 {
		return
	}
	if len(GConf.ConfigStore.Dir) == 0 {
		log.Printf("[ERROR]The ConfigStore's Dir must NOT be empty.")
		return
	}
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir(GConf.ConfigStore.Dir))
	mux.Handle("/", fs)
	mux.HandleFunc("/_conflist", getConfigList)
	err := http.ListenAndServe(GConf.ConfigStore.Listen, mux)
	if nil != err {
		log.Printf("[ERROR]Failed to start config store server:%v", err)
	}
}

var syncClient *http.Client

func syncConfigFile(addr string, localDir, remoteDir string, fileName string) error {
	filePath := remoteDir + "/" + fileName
	resp, err := syncClient.Get("http://" + addr + "/" + filePath)
	if nil != err {
		return err
	}
	if resp.StatusCode != 200 || nil == resp.Body {
		if nil != resp.Body {
			resp.Body.Close()
		}
		return fmt.Errorf("Invalid response:%v for %s", resp, filePath)
	}

	data, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	os.Mkdir(localDir+"/"+remoteDir, 0775)
	ioutil.WriteFile(localDir+"/"+filePath, data, 0660)
	log.Printf("Persistent config:%s.", localDir+"/"+filePath)
	return nil
}

func SyncConfig(addr string, localDir string) error {
	if nil == syncClient {
		tr := &http.Transport{}
		tr.ResponseHeaderTimeout = 2 * time.Second
		tr.Dial = netx.Dial
		syncClient = &http.Client{}
		syncClient.Timeout = 5 * time.Second
		syncClient.Transport = tr
	}
	resp, err := syncClient.Get("http://" + addr + "/_conflist")
	if nil != err {
		log.Printf("Error %v with local ip:%v", err, helper.GetLocalIPv4())
		return err
	}
	data, _ := ioutil.ReadAll(resp.Body)
	var confList []string
	json.Unmarshal(data, &confList)

	for _, conf := range confList {
		err = syncConfigFile(addr, localDir, conf, "client.json")
		if nil != err {
			return err
		}
		err = syncConfigFile(addr, localDir, conf, "hosts.json")
		if nil != err {
			return err
		}
		err = syncConfigFile(addr, localDir, conf, "apnic_cn.txt")
		if nil != err {
			return err
		}
		log.Printf("Synced config:%s success", conf)
	}
	return nil
}

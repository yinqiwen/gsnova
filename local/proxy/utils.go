package proxy

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/gfwlist"
)

var httpClientMap sync.Map

func NewHTTPClient(conf *ProxyChannelConfig) (*http.Client, error) {
	readTimeout := conf.ReadTimeout
	if 0 == readTimeout {
		readTimeout = 30
	}
	tr := &http.Transport{
		Dial:                  NewDialByConf(conf),
		DisableCompression:    true,
		MaxIdleConnsPerHost:   2 * int(conf.ConnsPerServer),
		ResponseHeaderTimeout: time.Duration(readTimeout) * time.Second,
	}
	if len(conf.SNI) > 0 {
		tlscfg := &tls.Config{}
		tlscfg.InsecureSkipVerify = true
		tlscfg.ServerName = conf.SNI[0]
		tr.TLSClientConfig = tlscfg
	}
	if len(conf.Proxy) > 0 {
		proxyUrl, err := url.Parse(conf.Proxy)
		if nil != err {
			log.Printf("[ERROR]Invalid proxy url:%s to create http client.", conf.Proxy)
			return nil, err
		}
		tr.Proxy = http.ProxyURL(proxyUrl)
	}
	hc := &http.Client{}
	//hc.Timeout = tr.ResponseHeaderTimeout
	hc.Transport = tr
	localClient, loaded := httpClientMap.LoadOrStore(conf, hc)
	if loaded {
		return localClient.(*http.Client), nil
	}
	return hc, nil
}

var syncGFWListTaskRunning = false
var syncIPRangeTaskRunning = false

func syncGFWList() {
	if syncGFWListTaskRunning {
		return
	}
	syncGFWListTaskRunning = true
	hc, _ := NewHTTPClient(&ProxyChannelConfig{Proxy: GConf.GFWList.Proxy})
	hc.Timeout = 30 * time.Second
	dst := proxyHome + "/gfwlist.txt"
	tmp, err := gfwlist.NewGFWList(GConf.GFWList.URL, hc, GConf.GFWList.UserRule, dst, true)
	if nil == err {
		mygfwlist = tmp
	} else {
		log.Printf("[ERROR]Failed to create gfwlist  for reason:%v", err)
	}
}

func syncIPRangeFile() {
	if syncIPRangeTaskRunning {
		return
	}
	syncIPRangeTaskRunning = true
	iprangeFile := proxyHome + "/" + cnIPFile
	ipHolder, err := parseApnicIPFile(iprangeFile)
	nextFetchTime := 1 * time.Second
	if nil == err {
		cnIPRange = ipHolder
		nextFetchTime = 1 * time.Minute
	}
	var hc *http.Client
	for {
		select {
		case <-time.After(nextFetchTime):
			if nil == hc {
				hc, err = NewHTTPClient(&ProxyChannelConfig{})
				hc.Timeout = 15 * time.Second
			}
			if nil != hc {
				ipHolder, err = getCNIPRangeHolder(hc)
				if nil != err {
					log.Printf("[ERROR]Failed to fetch CNIP file:%v", err)
					nextFetchTime = 1 * time.Second
				} else {
					log.Printf("Fetch latest IP range file success at %s", iprangeFile)
					nextFetchTime = 24 * time.Hour
					cnIPRange = ipHolder
				}
			}
		}
	}
}

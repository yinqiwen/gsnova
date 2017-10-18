package proxy

import (
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/gfwlist"
	"github.com/yinqiwen/gsnova/common/logger"
)

var httpClientMap sync.Map

func NewHTTPClient(conf *ProxyChannelConfig, scheme string) (*http.Client, error) {
	readTimeout := conf.ReadTimeout
	if 0 == readTimeout {
		readTimeout = 30
	}
	tr := &http.Transport{
		Dial:                  NewDialByConf(conf, scheme),
		DisableCompression:    true,
		MaxIdleConnsPerHost:   2 * int(conf.ConnsPerServer),
		ResponseHeaderTimeout: time.Duration(readTimeout) * time.Second,
	}
	// if len(conf.SNI) > 0 {
	// 	tlscfg := &tls.Config{}
	// 	tlscfg.InsecureSkipVerify = true
	// 	tlscfg.ServerName = conf.SNI[0]
	// 	tr.TLSClientConfig = tlscfg
	// }
	if len(conf.Proxy) > 0 {
		proxyUrl, err := url.Parse(conf.Proxy)
		if nil != err {
			logger.Error("[ERROR]Invalid proxy url:%s to create http client.", conf.Proxy)
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
	hc, _ := NewHTTPClient(&ProxyChannelConfig{Proxy: GConf.GFWList.Proxy}, "http")
	hc.Timeout = 30 * time.Second
	dst := proxyHome + "/gfwlist.txt"
	tmp, err := gfwlist.NewGFWList(GConf.GFWList.URL, hc, GConf.GFWList.UserRule, dst, true)
	if nil == err {
		mygfwlist = tmp
	} else {
		logger.Error("[ERROR]Failed to create gfwlist  for reason:%v", err)
	}
}

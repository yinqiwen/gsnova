package proxy

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
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

// func FillNOnce(auth *event.AuthEvent, nonceLen int) {
// 	auth.NOnce = make([]byte, nonceLen)
// 	io.ReadFull(rand.Reader, auth.NOnce)
// }

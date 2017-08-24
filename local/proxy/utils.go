package proxy

import (
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/local/hosts"
)

// func NewAuthEvent(secureTransport bool) *event.AuthEvent {
// 	auth := &event.AuthEvent{}
// 	auth.User = GConf.Auth
// 	//auth.Mac = getDeviceId()
// 	r := rand.New(rand.NewSource(time.Now().UnixNano()))
// 	auth.SetId(uint32(r.Int31()))
// 	auth.Rand = []byte(helper.RandAsciiString(int(r.Int31n(128))))
// 	if secureTransport && strings.EqualFold(GConf.Encrypt.Method, "auto") {
// 		auth.EncryptMethod = uint8(event.NoneEncrypter)
// 	} else {
// 		auth.EncryptMethod = event.GetDefaultCryptoMethod()
// 	}
// 	return auth
// }

func NewDialByConf(conf *ProxyChannelConfig) func(network, addr string) (net.Conn, error) {
	localDial := func(network, addr string) (net.Conn, error) {
		host, port, _ := net.SplitHostPort(addr)
		if port == "443" && len(conf.SNIProxy) > 0 && hosts.InHosts(conf.SNIProxy) {
			addr = hosts.GetAddr(conf.SNIProxy, "443")
			host, _, _ = net.SplitHostPort(addr)
		}
		if net.ParseIP(host) == nil {
			iphost, err := DnsGetDoaminIP(host)
			if nil != err {
				return nil, err
			}
			addr = net.JoinHostPort(iphost, port)
		}
		dailTimeout := conf.DialTimeout
		if 0 == dailTimeout {
			dailTimeout = 5
		}
		//log.Printf("Connect %s", addr)
		return netx.DialTimeout(network, addr, time.Duration(dailTimeout)*time.Second)
	}
	return localDial
}

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
	hc.Timeout = tr.ResponseHeaderTimeout
	hc.Transport = tr
	return hc, nil
}

// func FillNOnce(auth *event.AuthEvent, nonceLen int) {
// 	auth.NOnce = make([]byte, nonceLen)
// 	io.ReadFull(rand.Reader, auth.NOnce)
// }

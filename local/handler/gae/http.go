package gae

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/gsnova/local/proxy"
)

//var gaeHttpClient *http.Client

func initGAEClient(conf proxy.ProxyChannelConfig) *http.Client {
	client := new(http.Client)
	sslDial := func(n, addr string) (net.Conn, error) {
		var remote string
		var err error
		var conn net.Conn
		host, _, _ := net.SplitHostPort(addr)
		for i := 0; i < 3; i++ {
			remote = hosts.GetHost(host)
			timeout := conf.DialTimeout
			if 0 == timeout {
				timeout = 5
			}
			dialTimeout := time.Duration(timeout) * time.Second
			connAddr := addr
			if !strings.EqualFold(remote, host) {
				connAddr = remote + ":443"
			}
			if len(conf.Proxy) > 0 {
				conn, err = helper.HTTPProxyDial(conf.Proxy, connAddr, dialTimeout)
			} else {
				conn, err = netx.DialTimeout(n, connAddr, time.Duration(timeout)*time.Second)
			}

			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, err
		}
		tlcfg := &tls.Config{}
		tlcfg.InsecureSkipVerify = true
		sniLen := len(conf.SNI)
		if sniLen > 0 {
			tlcfg.ServerName = conf.SNI[rand.Intn(sniLen)]
		}
		tlsConn := tls.Client(conn, tlcfg)
		err = tlsConn.Handshake()
		if err != nil {
			log.Printf("TLS handshake error:%v", err)
			tlsConn.Close()
			return nil, err
		}
		return tlsConn, nil
	}
	readTimeout := conf.ReadTimeout
	if 0 == readTimeout {
		readTimeout = 15
	}
	tr := &http.Transport{
		Dial:                  sslDial,
		DisableCompression:    true,
		MaxIdleConnsPerHost:   int(conf.ConnsPerServer),
		ResponseHeaderTimeout: time.Duration(readTimeout) * time.Second,
	}
	client.Transport = tr
	return client
}

type httpChannel struct {
	server        string
	conf          proxy.ProxyChannelConfig
	gaeHttpClient *http.Client
}

func (hc *httpChannel) SetCryptoCtx(ctx *event.CryptoContext) {

}
func (hc *httpChannel) HandleCtrlEvent(ev event.Event) {

}

func (tc *httpChannel) ReadTimeout() time.Duration {
	readTimeout := tc.conf.ReadTimeout
	if 0 == readTimeout {
		readTimeout = 15
	}
	return time.Duration(readTimeout) * time.Second
}

func (h *httpChannel) Open() error {
	return nil
}

func (h *httpChannel) Closed() bool {
	return false
}

func (h *httpChannel) Close() error {
	return nil
}

func (h *httpChannel) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func (h *httpChannel) Request(p []byte) ([]byte, error) {
	var gaehost string
	if strings.Contains(h.server, ".") {
		gaehost = h.server
	} else {
		gaehost = h.server + ".appspot.com"
	}
	buf := bytes.NewBuffer(p)
	req := &http.Request{
		Method:        "POST",
		URL:           &url.URL{Scheme: "http", Host: gaehost, Path: "/invoke"},
		ProtoMajor:    1,
		ProtoMinor:    1,
		Host:          gaehost,
		Header:        make(http.Header),
		Body:          ioutil.NopCloser(buf),
		ContentLength: int64(buf.Len()),
	}
	req.Close = false
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "image/jpeg")
	if len(proxy.GConf.UserAgent) > 0 {
		req.Header.Set("User-Agent", proxy.GConf.UserAgent)
	}
	response, err := h.gaeHttpClient.Do(req)
	if nil != err {
		log.Printf("Failed to request data from GAE:%s", err)
		return nil, err
	} else {
		if nil != response.Body {
			defer response.Body.Close()
		}
		if response.StatusCode != 200 {
			log.Printf("Invalid response:%d", response.StatusCode)
			return nil, fmt.Errorf("Invalid response:%d", response.StatusCode)
		} else {
			var buf bytes.Buffer
			n, _ := io.Copy(&buf, response.Body)
			if int64(n) < response.ContentLength {
				return nil, fmt.Errorf("No sufficient space in body.")
			}
			return buf.Bytes(), nil
		}
	}
}

func (h *httpChannel) Write(p []byte) (n int, err error) {
	return 0, nil
}

func newHTTPChannel(server string, hc *http.Client, conf proxy.ProxyChannelConfig) (*proxy.RemoteChannel, error) {
	rc := &proxy.RemoteChannel{
		Addr:            server,
		Index:           0,
		DirectIO:        true,
		OpenJoinAuth:    false,
		WriteJoinAuth:   true,
		SecureTransport: true,
	}
	tc := new(httpChannel)
	tc.server = server
	tc.conf = conf
	tc.gaeHttpClient = hc
	rc.C = tc

	return rc, nil
}

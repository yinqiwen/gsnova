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
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/gsnova/local/proxy"
)

var gaeHttpClient *http.Client

func initGAEClient() error {
	if nil != gaeHttpClient {
		return nil
	}
	client := new(http.Client)

	sslDial := func(n, addr string) (net.Conn, error) {
		var remote string
		var err error
		var conn net.Conn
		host, _, _ := net.SplitHostPort(addr)
		for i := 0; i < 3; i++ {
			remote = hosts.GetHost(host)
			//log.Printf("SSL Dial %s:%s", remote, port)
			conn, err = netx.DialTimeout(n, remote+":443", 5*time.Second)
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, err
		}
		tlcfg := &tls.Config{}
		tlcfg.InsecureSkipVerify = true
		sniLen := len(proxy.GConf.GAE.SNI)
		if sniLen > 0 {
			tlcfg.ServerName = proxy.GConf.GAE.SNI[rand.Intn(sniLen)]
		}
		return tls.Client(conn, tlcfg), nil
	}

	tr := &http.Transport{
		Dial:                  sslDial,
		DisableCompression:    true,
		MaxIdleConnsPerHost:   int(proxy.GConf.GAE.ConnsPerServer),
		ResponseHeaderTimeout: 15 * time.Second,
	}
	if len(proxy.GConf.GAE.HTTPProxy) > 0 {
		proxyURL, err := url.Parse(proxy.GConf.GAE.HTTPProxy)
		if nil != err {
			return err
		}
		tr.Proxy = http.ProxyURL(proxyURL)
	}
	client.Transport = tr
	gaeHttpClient = client
	return nil
}

type httpChannel struct {
	server string
}

func (h *httpChannel) Open(iv uint64) error {
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
	response, err := gaeHttpClient.Do(req)
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

func newHTTPChannel(server string) (*proxy.RemoteChannel, error) {
	rc := &proxy.RemoteChannel{
		Addr:          server,
		Index:         0,
		DirectIO:      true,
		OpenJoinAuth:  false,
		WriteJoinAuth: true,
	}
	tc := new(httpChannel)
	tc.server = server
	rc.C = tc

	return rc, nil
}

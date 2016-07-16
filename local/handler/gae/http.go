package gae

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/gsnova/local/proxy"
)

var gaeHttpClient *http.Client

func initGAEClient() {
	if nil != gaeHttpClient {
		return
	}
	client := new(http.Client)
	tlcfg := &tls.Config{}
	tlcfg.InsecureSkipVerify = true

	sslDial := func(n, addr string) (net.Conn, error) {
		var remote string
		var err error
		var conn net.Conn
		host, port, _ := net.SplitHostPort(addr)
		for i := 0; i < 3; i++ {
			remote = hosts.GetHost(host)
			log.Printf("SSL Dial %s:%d", remote, port)
			conn, err = net.DialTimeout(n, remote, 5*time.Second)
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, err
		}
		return tls.Client(conn, tlcfg), nil
	}

	tr := &http.Transport{
		Dial:                  sslDial,
		TLSClientConfig:       tlcfg,
		DisableCompression:    true,
		MaxIdleConnsPerHost:   int(proxy.GConf.GAE.ConnsPerServer),
		ResponseHeaderTimeout: 15 * time.Second,
	}
	client.Transport = tr
	gaeHttpClient = client
}

type httpChannel struct {
	server string
}

func (h *httpChannel) Write(ev event.Event) (event.Event, error) {
	var buf bytes.Buffer
	auth := &event.AuthEvent{}
	auth.User = proxy.GConf.User
	event.EncodeEvent(&buf, auth)
	event.EncodeEvent(&buf, ev)
	var gaehost string
	if strings.Contains(h.server, ".") {
		gaehost = h.server
	} else {
		gaehost = h.server + ".appspot.com"
	}

	req := &http.Request{
		Method:        "POST",
		URL:           &url.URL{Scheme: "http", Host: gaehost, Path: "/invoke"},
		ProtoMajor:    1,
		ProtoMinor:    1,
		Host:          gaehost,
		Header:        make(http.Header),
		Body:          ioutil.NopCloser(&buf),
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
		if response.StatusCode != 200 {
			log.Printf("Session[%d]Invalid response:%d", ev.GetId(), response.StatusCode)
			return nil, fmt.Errorf("Invalid response:%d", response.StatusCode)
		} else {
			var buf bytes.Buffer
			n, err := io.Copy(&buf, response.Body)
			if int64(n) < response.ContentLength {
				return nil, fmt.Errorf("No sufficient space in body.")
			}
			if nil != err {
				return nil, err
			}
			response.Body.Close()
			err, res := event.DecodeEvent(&buf)
			return res, err
		}
	}

	return nil, nil
}

func newHTTPChannel(server string) *httpChannel {
	h := new(httpChannel)
	h.server = server
	return h
}

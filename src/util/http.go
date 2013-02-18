package util

import (
	"bufio"
	//"bytes"
	"crypto/tls"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	//"log"
)

func isKeepAlive(header http.Header, protoMajor, protoMinor int) bool {
	c := header.Get("Connection")
	if strings.EqualFold(c, "close") {
		return false
	}
	if protoMinor == 1 {
		return true
	}
	return strings.EqualFold(c, "keep-alive")
}

func IsRequestKeepAlive(req *http.Request) bool {
	if nil == req || req.Close {
		return false
	}
	return isKeepAlive(req.Header, req.ProtoMajor, req.ProtoMinor)
}

func IsResponseKeepAlive(res *http.Response) bool {
	if nil == res || res.Close || (res.ContentLength == -1 && len(res.TransferEncoding) == 0) {
		return false
	}
//	if res.StatusCode == 301 || res.StatusCode == 302 {
//		return false
//	}
	ret := isKeepAlive(res.Header, res.ProtoMajor, res.ProtoMinor)
	//	if ret && res.ContentLength == 0 && len(res.Header.Get("Connection")) == 0 {
	//		return false
	//	}
	return ret
}

func FetchLateastContent(urlstr string, proxy_port string, cmp time.Time, force bool) ([]byte, string, error) {
	resp, err := HttpGet(urlstr, "")
	if err != nil {
		resp, err = HttpGet(urlstr, "http://"+net.JoinHostPort("127.0.0.1", proxy_port))
	}
	if err != nil || resp.StatusCode != 200 {
		return nil, "", err
	} else {
		last_mod_date := resp.Header.Get("last-modified")
		if !force && len(last_mod_date) > 0 {
			//return nil, "", errors.New("No last-modified header in response.")
			t, err := time.Parse(time.RFC1123, last_mod_date)
			if nil == err && t.Before(cmp) {
				resp.Body.Close()
				//log.Printf("###########%v, %v for %s\n", t, cmp, urlstr)
				return []byte{}, last_mod_date, nil
			}
		}

		if nil != err {
			return nil, last_mod_date, err
		}
		body, err := ioutil.ReadAll(resp.Body)
		if nil == err {
			return body, last_mod_date, nil
		}
	}
	return nil, "", errors.New("Invalid url")
}

func HttpGet(urlstr string, proxy string) (*http.Response, error) {
	if len(proxy) == 0 {
		return http.Get(urlstr)
	}
	tr := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			return url.Parse(proxy)
		},
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}
	return client.Get(urlstr)
}

func HttpTunnelDial(network, addr string, tunnel_url *url.URL) (c net.Conn, err error) {
	if nil == tunnel_url {
		return net.Dial(network, addr)
	}
	c, err = net.Dial("tcp", tunnel_url.Host)
	if nil != err {
		return
	}
	reader := bufio.NewReader(c)
	req := &http.Request{
		Method: "CONNECT",
		URL:    &url.URL{Host: addr},
		Host:   addr,
		Header: make(http.Header),
	}
	err = req.Write(c)
	if nil != err {
		return
	}
	var res *http.Response
	res, err = http.ReadResponse(reader, req)
	if nil != err {
		return
	}
	if res.StatusCode >= 300 {
		c.Close()
		return nil, errors.New(res.Status)
	}
	return
}

//type ChunkBody struct {
//	ch       chan []byte
//	cumulate *bytes.Buffer
//	closed   bool
//}
//
//func (body *ChunkBody) Read(p []byte) (n int, err error) {
//	if body.closed {
//		return 0, io.EOF
//	}
//	var b []byte
//	if body.cumulate.Len() > 0{
//	   b = body.cumulate.Bytes()
//	   body.cumulate.Reset()
//	}else{
//	  b = <-body.ch
//	}
//	if nil == b || len(b) == 0 {
//		body.closed = true
//		//log.Printf("#####Closed\n")
//		return 0, io.EOF
//	}
//	k := copy(p, b)
//	if k < len(b) {
//		body.cumulate.Write(b[k:])
//	}else{
//	}
//	return k, nil
//}
//
//func (body *ChunkBody) Offer(p []byte) error {
//	if body.closed {
//		return io.EOF
//	}
//	body.ch <- p
//	return nil
//}
//
//func (body *ChunkBody) Close() error {
//	//body.closed = true
//	body.ch <- nil
//	return nil
//}
//
//func NewChunkBody() *ChunkBody {
//	body := new(ChunkBody)
//	body.ch = make(chan []byte, 100)
//	body.cumulate = new(bytes.Buffer)
//	body.closed = false
//	return body
//}

type DelegateConnListener struct {
	connChan chan net.Conn
}

func (d *DelegateConnListener) Accept() (c net.Conn, err error) {
	select {
	case c = <-d.connChan:
		if nil != c {
			return
		}
	}
	return nil, errors.New("No connection")
}

func (d *DelegateConnListener) Close() error {
	//do nothing
	return nil
}

func (d *DelegateConnListener) Addr() net.Addr {
	return nil
}

func (d *DelegateConnListener) Delegate(c net.Conn, req *http.Request) {
	a, b := net.Pipe()
	d.connChan <- b
	req.Write(a)
	go func() {
		io.Copy(c, a)
		a.Close()
		b.Close()
		c.Close()
	}()
}

func NewDelegateConnListener() *DelegateConnListener {
	d := new(DelegateConnListener)
	d.connChan = make(chan net.Conn)
	return d
}

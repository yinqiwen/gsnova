package util

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
)

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

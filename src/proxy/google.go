package proxy

import (
	"bytes"
	"common"
	"crypto/tls"
	"event"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
	"util"
)

const (
	GOOGLE_HTTPS_IP = "GoogleHttpsIP"
	GOOGLE_HTTPS    = "GoogleHttps"
	GOOGLE_HTTP_IP  = "GoogleHttpIP"
	GOOGLE_HTTP     = "GoogleHttp"
)

var preferIP = true
var googleHttpHost = "GoogleHttpIP"
var googleHttpsHost = "GoogleHttps"
var connTimeoutSecs time.Duration
var googleLocalProxy string

var httpGoogleManager *Google
var httpsGoogleManager *Google
var google_enable bool
var total_google_conn_num int32
var total_google_routine_num int32

var httpGoogleClient *http.Client
var httpsGoogleClient *http.Client

type GoogleConnection struct {
	proxyAddr   string
	proxyURL    *url.URL
	overProxy   bool
	manager     *Google
	simple_url  bool
	use_sys_dns bool
}

func (conn *GoogleConnection) IsDisconnected() bool {
	return false
}

func (conn *GoogleConnection) Close() error {
	return nil
}

func (conn *GoogleConnection) GetConnectionManager() RemoteConnectionManager {
	return conn.manager
}

func (google *GoogleConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	f := func(local, remote net.Conn, ch chan int) {
		io.Copy(remote, local)
		ch <- 1
		local.Close()
		remote.Close()
	}
	//L:
	switch ev.GetType() {

	case event.HTTP_REQUEST_EVENT_TYPE:
		req := ev.(*event.HTTPRequestEvent)
		if conn.Type == HTTPS_TUNNEL {
			var proxyConn net.Conn
			if len(googleLocalProxy) > 0 {
				proxyURL, _ := url.Parse(googleLocalProxy)
				proxyConn, err = net.Dial("tcp", proxyURL.Host)
				addr, _ := getLocalHostMapping(GOOGLE_HTTPS)

				connreq := req.RawReq
				connreq.Host = addr
				if nil == err {
					connreq.Write(proxyConn)
				}
			} else {
				addr := getGoogleHostport(true)
				proxyConn, err = net.DialTimeout("tcp", addr, connTimeoutSecs)
				if nil != err {
					//try again
					addr = getGoogleHostport(true)
					proxyConn, err = net.DialTimeout("tcp", addr, connTimeoutSecs)
				}
			}

			log.Printf("Session[%d]Request %s\n", req.GetHash(), util.GetURLString(req.RawReq, true))
			if nil == err {
				if len(googleLocalProxy) > 0 {
				} else {
					conn.LocalRawConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
				}
			} else {
				return fmt.Errorf("No google proxy reachable:%v", err), nil
			}
			ch := make(chan int)
			go f(conn.LocalRawConn, proxyConn, ch)
			go f(proxyConn, conn.LocalRawConn, ch)
			atomic.AddInt32(&total_google_routine_num, 2)
			<-ch
			<-ch
			atomic.AddInt32(&total_google_routine_num, -2)
			proxyConn.Close()
			google.Close()
			conn.State = STATE_SESSION_CLOSE
		} else {
			google.proxyAddr = req.RawReq.Host
			log.Printf("Session[%d]Request %s\n", req.GetHash(), util.GetURLString(req.RawReq, true))
			req.RawReq.URL.Scheme = "http"
			req.RawReq.RequestURI = ""
			var resp *http.Response
			tryProxy := func() (*http.Response, error) {
				if google.manager == httpGoogleManager {
					return httpGoogleClient.Do(req.RawReq)
				}
				return httpsGoogleClient.Do(req.RawReq)
			}
			resp, err = tryProxy()
			if nil != err && strings.EqualFold(req.Method, "GET") {
				//try proxy again
				resp, err = tryProxy()
			}
			if nil != err {
				var tmp bytes.Buffer
				req.RawReq.Write(&tmp)
				log.Printf("Session[%d]Request error:%v\n%s\n", req.GetHash(), err, tmp.String())
				return err, nil
			}
			err = resp.Write(conn.LocalRawConn)
			if nil != err || !util.IsResponseKeepAlive(resp) || !util.IsRequestKeepAlive(req.RawReq) {
				conn.LocalRawConn.Close()
				conn.State = STATE_SESSION_CLOSE
			} else {
			    log.Printf("Session[%d]Res %d %v\n", req.GetHash(), resp.StatusCode, resp.Header)
				conn.State = STATE_RECV_HTTP
			}
		}
	default:
	}
	return nil, nil
}

type Google struct {
	name string
}

func (manager *Google) GetName() string {
	return manager.name
}

func (manager *Google) RecycleRemoteConnection(conn RemoteConnection) {
	atomic.AddInt32(&total_google_conn_num, -1)
}

func (manager *Google) GetRemoteConnection(ev event.Event, attrs map[string]string) (RemoteConnection, error) {
	g := new(GoogleConnection)
	g.manager = manager
	if containsAttr(attrs, ATTR_DIRECT) {
		g.simple_url = true
	} else {
		g.simple_url = false
	}
	if containsAttr(attrs, ATTR_SYS_DNS) {
		g.use_sys_dns = true
	} else {
		g.use_sys_dns = false
	}
	atomic.AddInt32(&total_google_conn_num, 1)
	return g, nil
}

func getGoogleHostport(isHttps bool) string {
	var addr string
	if !isHttps {
		addr, _ = getLocalHostMapping(googleHttpHost)
		addr = net.JoinHostPort(addr, "80")

	} else {
		addr, _ = getLocalHostMapping(googleHttpsHost)
		addr = net.JoinHostPort(addr, "443")
	}
	if !preferIP {
		addr, _ = lookupAvailableAddress(addr, true)
	}
	return addr
}

func initGoogleHttpClients() {
	httpGoogleClient = new(http.Client)
	httpsGoogleClient = new(http.Client)

	tlcfg := &tls.Config{}
	tlcfg.InsecureSkipVerify = true
	commonDial := func(n, addr string, isHttps bool) (net.Conn, error) {
		remote := getGoogleHostport(isHttps)
		log.Printf("Connect google server:%s\n", remote)
		conn, err := net.DialTimeout(n, remote, connTimeoutSecs)
		if err != nil {
			expireBlockVerifyCache(remote)
			//try again
			remote = getGoogleHostport(isHttps)
			conn, err = net.DialTimeout(n, remote, connTimeoutSecs)
		}
		if err != nil {
			expireBlockVerifyCache(addr)
			return nil, err
		}
		if isHttps {
			return tls.Client(conn, tlcfg), nil
		}
		return conn, err
	}

	httpDial := func(n, addr string) (net.Conn, error) {
		return commonDial(n, addr, false)
	}

	httpsDial := func(n, addr string) (net.Conn, error) {
		return commonDial(n, addr, true)
	}

	proxyFunc := func(req *http.Request) (*url.URL, error) {
		if len(googleLocalProxy) > 0 {
			proxy := getLocalUrlMapping(googleLocalProxy)
			log.Printf("Google use proxy:%s\n", proxy)
			return url.Parse(proxy)
		}
		//just a trick to let http client to write proxy request
		return url.Parse("http://localhost:48100")
	}
	if len(googleLocalProxy) > 0 {
		httpDial = net.Dial
		httpsDial = net.Dial
	}

	httpGoogleClient.Transport = &http.Transport{
		DisableCompression:  true,
		Dial:                httpDial,
		Proxy:               proxyFunc,
		MaxIdleConnsPerHost: 20,
		ResponseHeaderTimeout: 10 * time.Second,
	}

	httpsGoogleClient.Transport = &http.Transport{
		DisableCompression:  true,
		Dial:                httpsDial,
		Proxy:               proxyFunc,
		MaxIdleConnsPerHost: 20,
		ResponseHeaderTimeout: 10 * time.Second,
	}
}

func InitGoogle() error {
	if enable, exist := common.Cfg.GetIntProperty("Google", "Enable"); exist {
		google_enable = (enable != 0)
		if enable == 0 {
			return nil
		}
	}
	
	if prefer, exist := common.Cfg.GetBoolProperty("Google", "PreferIP"); exist {
		preferIP = prefer
	}
	if preferIP {
		googleHttpsHost = GOOGLE_HTTPS_IP
		googleHttpHost = GOOGLE_HTTP_IP
	} else {
		googleHttpsHost = GOOGLE_HTTPS
		googleHttpHost = GOOGLE_HTTP
	}
	connTimeoutSecs = 1500 * time.Millisecond
	if tmp, exist := common.Cfg.GetIntProperty("Google", "ConnectTimeout"); exist {
		connTimeoutSecs = time.Duration(tmp) * time.Millisecond
	}
	if proxy, exist := common.Cfg.GetProperty("Google", "Proxy"); exist {
		googleLocalProxy = proxy
	}
	httpGoogleManager = newGoogle(GOOGLE_HTTP_NAME)
	httpsGoogleManager = newGoogle(GOOGLE_HTTPS_NAME)
	initGoogleHttpClients()
	log.Println("Init Google Module Success")
	return nil
}

func newGoogle(name string) *Google {
	manager := new(Google)
	manager.name = name
	return manager
}

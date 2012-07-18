package paas

import (
	"bufio"
	"bytes"
	"common"
	"crypto/tls"
	"errors"
	"event"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"util"
)

const (
	ANONYMOUSE = "anonymouse"
)

type GAEConfig struct {
	Compressor           uint32
	Encrypter            uint32
	InjectRange          []string
	UA                   string
	ConnectionMode       string
	ConnectionPoolSize   uint32
	FetchLimitSize       uint32
	RangeFetchRetryLimit uint32
	MasterAppID          string
}

var gae_cfg *GAEConfig

type GAEAuth struct {
	appid  string
	user   string
	passwd string
	token  string
}

func (auth *GAEAuth) parse(line string) error {
	line = strings.TrimSpace(line)
	v := strings.Split(line, "@")
	if len(v) == 1 {
		auth.appid = line
		auth.user = ANONYMOUSE
		auth.passwd = ANONYMOUSE
	} else if len(v) == 2 {
		userpass := v[0]
		auth.appid = v[1]
		us := strings.Split(userpass, ":")
		if len(us) != 2 {
			return errors.New("Invalid user/pass: " + userpass)
		}
		auth.user = us[0]
		auth.passwd = us[1]
	} else {
		return errors.New("Invalid user/pass/appid: " + line)
	}
	return nil
}

type GAEHttpConnection struct {
	auth        GAEAuth
	authToken   string
	client      *http.Client
	remote_conn net.Conn
	manager     *GAE
	rangeStart  int
}

func (conn *GAEHttpConnection) GetConnectionManager() RemoteConnectionManager {
	return conn.manager
}

func (conn *GAEHttpConnection) initHttpClient() {
	if nil != conn.client {
		return
	}
	conn.client = new(http.Client)
	tlcfg := &tls.Config{}
	tlcfg.InsecureSkipVerify = true
	var dial func(n, addr string) (net.Conn, error)
	sslDial := func(n, addr string) (net.Conn, error) {
		conn, err := net.Dial(n, addr)
		if err != nil {
			return nil, err
		}
		return tls.Client(conn, tlcfg), nil
	}
	proxyInfo, exist := common.Cfg.GetProperty("LocalProxy", "Proxy")
	if exist {
		log.Printf("GAE use proxy:%s\n", util.GetUrl(proxyInfo))
		if strings.HasPrefix(proxyInfo, "https") {
			dial = sslDial
		}
		tr := &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				proxy := util.GetUrl(proxyInfo)
				proxyURL, err := url.Parse(proxy)
				if err != nil || proxyURL.Scheme == "" {
					if u, err := url.Parse("http://" + proxy); err == nil {
						proxyURL = u
						err = nil
					}
				}
				if err != nil {
					return nil, fmt.Errorf("invalid proxy address %q: %v", proxy, err)
				}
				//fmt.Println(proxyURL)
				return proxyURL, nil
			},
			Dial:               dial,
			TLSClientConfig:    tlcfg,
			DisableCompression: true,
		}
		conn.client.Transport = tr
	} else {
		if mode, exist := common.Cfg.GetProperty("GAE", "ConnectionMode"); exist {
			if strings.EqualFold(mode, "https") {
				dial = sslDial
			}
		}
		tr := &http.Transport{
			Dial:               dial,
			TLSClientConfig:    tlcfg,
			DisableCompression: true,
		}
		conn.client.Transport = tr
	}
}

func (conn *GAEHttpConnection) Auth() error {
	conn.initHttpClient()
	var authEvent event.AuthRequestEvent
	authEvent.User = conn.auth.user
	authEvent.Passwd = conn.auth.passwd
	authEvent.Appid = conn.auth.appid
	err, res := conn.Request(nil, &authEvent)
	if nil != err {
		log.Println(err)
		return err
	}

	if authres, ok := res.(*event.AuthResponseEvent); !ok {
		log.Printf("Type is  %d\n", res.GetType())
		return errors.New("Invalid auth response.")
	} else {
		log.Printf("Auth token is %s\n", authres.Token)
		conn.authToken = authres.Token
	}
	return nil
}

func (gae *GAEHttpConnection) requestEvent(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	gae.initHttpClient()
	domain := gae.auth.appid + ".appspot.com"
	addr := util.GetHost(domain)
	scheme := "http"
	if strings.EqualFold("https", gae_cfg.ConnectionMode) {
		scheme = "https"
	}
	var buf bytes.Buffer
	var tags event.EventHeaderTags
	tags.Token = gae.authToken
	tags.Encode(&buf)
	if ev.GetType() == event.HTTP_REQUEST_EVENT_TYPE {
		var compress event.CompressEvent
		compress.SetHash(ev.GetHash())
		compress.Ev = ev
		compress.CompressType = gae_cfg.Compressor
		var encrypt event.EncryptEvent
		encrypt.SetHash(ev.GetHash())
		encrypt.EncryptType = gae_cfg.Encrypter
		encrypt.Ev = &compress
		event.EncodeEvent(&buf, &encrypt)
	} else {
		var encrypt event.EncryptEvent
		encrypt.SetHash(ev.GetHash())
		encrypt.EncryptType = gae_cfg.Encrypter
		encrypt.Ev = ev
		event.EncodeEvent(&buf, &encrypt)
	}
	req := &http.Request{
		Method:        "POST",
		URL:           &url.URL{Scheme: scheme, Host: addr, Path: "/invoke"},
		Host:          addr,
		Header:        make(http.Header),
		Body:          ioutil.NopCloser(&buf),
		ContentLength: int64(buf.Len()),
	}

	if len(gae_cfg.UA) > 0 {
		req.Header.Set("User-Agent", gae_cfg.UA)
	}
	req.Close = false
	req.Header.Set("Connection", "close")
	req.Header.Set("Content-Type", "application/octet-stream")
	//log.Println(gae.auth)
	if response, err := gae.client.Do(req); nil != err {
		log.Printf("Failed to request data from GAE:%s\n", err.Error())
		return err, nil
	} else {
		if response.StatusCode != 200 {
			log.Printf("Invalid response:%d\n", response.StatusCode)
			return errors.New("Invalid response"), nil
		} else {
			//log.Printf("Response with content len:%d\n", response.ContentLength)
			content := make([]byte, response.ContentLength)
			_, err := io.ReadFull(response.Body, content)
			if nil != err {
				return err, nil
			}
			buf := bytes.NewBuffer(content)
			if !tags.Decode(buf) {
				return errors.New("Failed to decode event tag"), nil
			}
			err, res = event.DecodeEvent(buf)
			if nil == err {
				for res.GetType() == event.ENCRYPT_EVENT_TYPE || res.GetType() == event.COMPRESS_EVENT_TYPE {
					encrypt, ok := res.(*event.EncryptEvent)
					if ok {
						res = encrypt.Ev
					}
					compress, ok := res.(*event.CompressEvent)
					if ok {
						res = compress.Ev
					}
				}
			}
			buf.Reset()
			buf = nil
			return err, res
		}
	}
	return nil, nil
}

func (gae *GAEHttpConnection) handleHttpRes(conn *SessionConnection, req *event.HTTPRequestEvent, ev *event.HTTPResponseEvent) error {
	contentRange := ev.GetHeader("Content-Range")
	if len(contentRange) > 0 {
		httpres := ev.ToResponse()
		httpres.Header.Del("Content-Range")
		startpos, endpos, length := util.ParseContentRangeHeaderValue(contentRange)
		httpres.ContentLength = int64(length - gae.rangeStart)
		httpres.Write(conn.LocalRawConn)
		for endpos < length-1 {
			startpos = endpos + 1
			endpos = endpos + int(gae_cfg.FetchLimitSize)
			if endpos >= length-1 {
				endpos = length - 1
			}
			req.SetHeader("Range", fmt.Sprintf("bytes=%d-%d", startpos, endpos))
			err, rangeres := gae.requestEvent(nil, req)
			//try again
			if nil != err {
				err, rangeres = gae.requestEvent(nil, req)
			}
			if nil != err {
				return err
			}
			rangeHttpRes := rangeres.(*event.HTTPResponseEvent)
			if rangeHttpRes.Status == 302 {
				location := rangeHttpRes.GetHeader("Location")
				xrange := rangeHttpRes.GetHeader("X-Range")
				if len(location) > 0 && len(xrange) > 0 {
					req.Url = location
					req.SetHeader("Range", xrange)
					err, rangeres = gae.requestEvent(nil, req)
					if nil != err {
						return err
					}
				}
			}
			_, err = conn.LocalRawConn.Write(rangeres.(*event.HTTPResponseEvent).Content.Bytes())
			rangeres.(*event.HTTPResponseEvent).Content.Reset()
			rangeres = nil
			if nil != err {
				return err
			}
		}
		return nil
	}

	httpres := ev.ToResponse()
	//log.Println(httpres.Header)
	err := httpres.Write(conn.LocalRawConn)
	httpres = nil
	return err
}

func (gae *GAEHttpConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	if ev.GetType() == event.HTTP_REQUEST_EVENT_TYPE {
		httpreq := ev.(*event.HTTPRequestEvent)
		if strings.EqualFold(httpreq.Method, "CONNECT") {
			log.Printf("Request %s %s\n", httpreq.Method, httpreq.Url)
			conn.LocalRawConn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
			tlscfg, err := common.TLSConfig(httpreq.GetHeader("Host"))
			if nil != err {
				return err, nil
			}
			conn.LocalRawConn = tls.Server(conn.LocalRawConn, tlscfg)
			conn.LocalBufferConn = bufio.NewReader(conn.LocalRawConn)
			conn.State = STATE_RECV_HTTP
			conn.Type = HTTPS_TUNNEL
			return nil, nil
		} else {
			if httpreq.Content.Len() == 0 {
				body := make([]byte, httpreq.RawReq.ContentLength)
				io.ReadFull(httpreq.RawReq.Body, body)
				httpreq.Content.Write(body)
			}
			scheme := "http://"
			if conn.Type == HTTPS_TUNNEL {
				scheme = "https://"
			}
			if !strings.HasPrefix(httpreq.Url, scheme) {
				httpreq.Url = scheme + httpreq.RawReq.Host + httpreq.Url
			}
			gae.rangeStart = 0
			rangeHeader := httpreq.GetHeader("Range")
			if len(rangeHeader) > 0 {
				startPos, endPos := util.ParseRangeHeaderValue(rangeHeader)
				if endPos == -1 || endPos-startPos > int(gae_cfg.FetchLimitSize-1) {
					endPos = startPos + int(gae_cfg.FetchLimitSize-1)
					httpreq.SetHeader("Range", fmt.Sprintf("bytes=%d-%d", startPos, endPos))
					gae.rangeStart = startPos
				}
			} else {
				//inject range header
				for _, host_pattern := range gae_cfg.InjectRange {
					if strings.Contains(httpreq.RawReq.Host, host_pattern) {
						httpreq.SetHeader("Range", "bytes=0-"+strconv.Itoa(int(gae_cfg.FetchLimitSize-1)))
						break
					}
				}
			}

			log.Printf("Request %s %s\n", httpreq.Method, httpreq.Url)
			//httpreq.SetHeader("Connection", "Close")
			err, res = gae.requestEvent(conn, ev)
			if nil != err {
				return
			}
			conn.State = STATE_RECV_HTTP
			if nil != conn {
				httpres := res.(*event.HTTPResponseEvent)
				gae.handleHttpRes(conn, httpreq, httpres)
				//if !httpres.IsKeepAlive(){
				if conn.Type == HTTPS_TUNNEL {
					conn.LocalRawConn.Close()
					conn.State = STATE_SESSION_CLOSE
				}
				//gae.manager.RecycleRemoteConnection(gae)
			}
			return nil, nil
		}
	}
	return gae.requestEvent(conn, ev)
	//	err, res := gae.requestEvent(conn, ev)
	//	if nil != err {
	//	}
}

type GAE struct {
	auths      *util.ListSelector
	idle_conns chan RemoteConnection
}

func (manager *GAE) RecycleRemoteConnection(conn RemoteConnection) {
	select {
	case manager.idle_conns <- conn:
		// Buffer on free list; nothing more to do.
	default:
		// Free list full, just carry on.
	}
}

func (manager *GAE) GetRemoteConnection(ev event.Event) (RemoteConnection, error) {
	var b RemoteConnection
	// Grab a buffer if available; allocate if not.
	select {
	case b = <-manager.idle_conns:
		// Got one; nothing more to do.
	default:
		// None free, so allocate a new one.
		gae := new(GAEHttpConnection)
		gae.auth = *(manager.auths.Select().(*GAEAuth))
		gae.authToken = gae.auth.token
		gae.manager = manager
		b = gae
		//b.auth = 
	} // Read next message from the net.
	return b, nil
}

func (manager *GAE) GetName() string {
	return GAE_NAME
}

func initConfig() {
	//init config
	gae_cfg = new(GAEConfig)
	if ua, exist := common.Cfg.GetProperty("GAE", "UserAgent"); exist {
		gae_cfg.UA = ua
	}
	gae_cfg.ConnectionMode = "HTTP"
	if cm, exist := common.Cfg.GetProperty("GAE", "ConnectionMode"); exist {
		gae_cfg.ConnectionMode = cm
	}
	gae_cfg.Compressor = event.COMPRESSOR_SNAPPY
	if compress, exist := common.Cfg.GetProperty("GAE", "Compressor"); exist {
		if strings.EqualFold(compress, "None") {
			gae_cfg.Compressor = event.COMPRESSOR_NONE
		}
	}
	gae_cfg.Encrypter = event.ENCRYPTER_SE1
	if compress, exist := common.Cfg.GetProperty("GAE", "Encrypter"); exist {
		if strings.EqualFold(compress, "None") {
			gae_cfg.Compressor = event.ENCRYPTER_NONE
		}
	}
	gae_cfg.ConnectionPoolSize = 20
	if poosize, exist := common.Cfg.GetIntProperty("GAE", "ConnectionPoolSize"); exist {
		gae_cfg.ConnectionPoolSize = uint32(poosize)
	}
	//	gae_cfg.ConcurrentRangeFetcher = 3
	//	if fetcher, exist := common.Cfg.GetIntProperty("GAE", "ConcurrentRangeFetcher"); exist {
	//		gae_cfg.ConcurrentRangeFetcher = uint32(fetcher)
	//	}
	gae_cfg.FetchLimitSize = 256000
	if limit, exist := common.Cfg.GetIntProperty("GAE", "FetchLimitSize"); exist {
		gae_cfg.FetchLimitSize = uint32(limit)
	}
	gae_cfg.RangeFetchRetryLimit = 1
	if limit, exist := common.Cfg.GetIntProperty("GAE", "RangeFetchRetryLimit"); exist {
		gae_cfg.RangeFetchRetryLimit = uint32(limit)
	}
	if ranges, exist := common.Cfg.GetProperty("GAE", "InjectRange"); exist {
		gae_cfg.InjectRange = strings.Split(ranges, "|")
	}
	gae_cfg.MasterAppID = "snova-master"
	if master, exist := common.Cfg.GetProperty("GAE", "MasterAppID"); exist {
		gae_cfg.MasterAppID = master
	}
}

func (manager *GAE) fetchSharedAppIDs() (error, []string) {
	var auth GAEAuth
	auth.appid = gae_cfg.MasterAppID
	auth.user = ANONYMOUSE
	auth.passwd = ANONYMOUSE
	conn := new(GAEHttpConnection)
	conn.auth = auth
	conn.manager = manager
	var req event.RequestAppIDEvent
	err, res := conn.Request(nil, &req)
	if nil != err {
		return err, nil
	}
	appidres, ok := res.(*event.RequestAppIDResponseEvent)
	if ok {
		return nil, appidres.AppIDs
	}
	return errors.New("Invalid response for shared appid."), nil
}

func (manager *GAE) Init() error {
	log.Println("Init GAE.")
	RegisteRemoteConnManager(manager)
	initConfig()
	manager.idle_conns = make(chan RemoteConnection, gae_cfg.ConnectionPoolSize)
	manager.auths = new(util.ListSelector)
	index := 0
	for {
		v, exist := common.Cfg.GetProperty("GAE", "WorkerNode["+strconv.Itoa(index)+"]")
		if !exist || len(v) == 0 {
			break
		}
		var auth GAEAuth
		if err := auth.parse(v); nil != err {
			return err
		}
		manager.auths.Add(&auth)
		index = index + 1
	}
	//no appid found, fetch shared from master
	if index == 0 {
		err, appids := manager.fetchSharedAppIDs()
		if nil != err {
			return err
		}
		for _, appid := range appids {
			//log.Printf("Fetched appid:%s\n", appid)
			var auth GAEAuth
			auth.appid = appid
			auth.user = ANONYMOUSE
			auth.passwd = ANONYMOUSE
			manager.auths.Add(&auth)
		}
	}
	for _, au := range manager.auths.ArrayValues() {
		auth := au.(*GAEAuth)
		conn := new(GAEHttpConnection)
		conn.auth = *auth
		conn.manager = manager
		if err := conn.Auth(); nil != err {
			log.Printf("Failed to auth appid:%s\n", err.Error())
			return err
		}
		auth.token = conn.authToken
		conn.auth.token = conn.authToken
		//manager.auths.Add(&auth)
		manager.RecycleRemoteConnection(conn)
	}
	return nil
}

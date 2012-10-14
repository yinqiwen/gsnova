package proxy

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
	"regexp"
	"strconv"
	"strings"
	"time"
	"util"
)

const (
	ANONYMOUSE = "anonymouse"
)

type GAEConfig struct {
	Compressor             uint32
	Encrypter              uint32
	InjectRange            []*regexp.Regexp
	UA                     string
	ConnectionMode         string
	ConnectionPoolSize     uint32
	FetchLimitSize         uint32
	RangeFetchRetryLimit   uint32
	MasterAppID            string
	ConcurrentRangeFetcher uint32
}

var gae_cfg *GAEConfig
var gae_enable bool
var gae_use_shared_appid bool

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
	auth               GAEAuth
	authToken          string
	client             *http.Client
	remote_conn        net.Conn
	manager            *GAE
	proxyURL           *url.URL
	rangeStart         int
	rangeFetchChannel  chan *rangeChunk
	range_expected_pos int
	closed             bool
}

func (gae *GAEHttpConnection) Close() error {
	gae.closed = true
	return nil
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

	dial := func(n, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(n, addr, 2*time.Second)
		if err != nil {
			return nil, err
		}
		return conn, err
	}

	sslDial := func(n, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(n, addr, 2*time.Second)
		if err != nil {
			return nil, err
		}
		return tls.Client(conn, tlcfg), nil
	}
	proxyInfo, exist := common.Cfg.GetProperty("LocalProxy", "Proxy")
	if exist {
		proxy := util.GetUrl(proxyInfo)
		if strings.HasPrefix(proxyInfo, "https") {
			dial = sslDial
		}
		tr := &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				if nil != conn.proxyURL {
					return conn.proxyURL, nil
				}
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
				conn.proxyURL = proxyURL
				return proxyURL, nil
			},
			Dial:               dial,
			TLSClientConfig:    tlcfg,
			DisableCompression: true,
		}
		conn.client.Transport = tr
	} else {
		if mode, exist := common.Cfg.GetProperty("GAE", "ConnectionMode"); exist {
			if strings.EqualFold(mode, MODE_HTTPS) {
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
	auth := &gae.auth

	domain := auth.appid + ".appspot.com"
	if strings.Contains(auth.appid, ".") {
		domain = auth.appid
	}
	addr, _ := util.GetHostMapping(domain)
	scheme := MODE_HTTP
	if strings.EqualFold(MODE_HTTPS, gae_cfg.ConnectionMode) {
		scheme = MODE_HTTPS
	}
	var buf bytes.Buffer
	var tags event.EventHeaderTags
	tags.Token = auth.token
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
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "image/jpeg")
	var response *http.Response
	response, err = gae.client.Do(req)
	if nil != err {
		log.Printf("Failed to request data from GAE:%s for:%s\n", domain, err.Error())
		return err, nil
	} else {
		if response.StatusCode != 200 {
			log.Printf("Session[%d]Invalid response:%d\n", ev.GetHash(), response.StatusCode)
			return errors.New("Invalid response"), nil
		} else {
			//log.Printf("Response with content len:%d\n", response.ContentLength)
			content := make([]byte, response.ContentLength)
			n, err := io.ReadFull(response.Body, content)
			if int64(n) < response.ContentLength {
				return errors.New("No sufficient space in body."), nil
			}
			if nil != err {
				return err, nil
			}
			response.Body.Close()

			buf := bytes.NewBuffer(content[0:response.ContentLength])
			if !tags.Decode(buf) {
				return errors.New("Failed to decode event tag"), nil
			}
			err, res = event.DecodeEvent(buf)
			if nil == err {
				res = event.ExtractEvent(res)
			}
			return err, res
		}
	}

	return nil, nil
}

func (gae *GAEHttpConnection) rangeFetch(req *event.HTTPRequestEvent, index, startpos, limit int) {
	clonereq := req.DeepClone()
	for startpos < limit-1 && !gae.closed {
		if startpos-gae.range_expected_pos >= 1024*1024 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		endpos := startpos + int(gae_cfg.FetchLimitSize) - 1
		if endpos > limit {
			endpos = limit
		}
		log.Printf("Session[%d]Fetch[%d] range chunk[%d:%d-%d]from %s", req.GetHash(), index, startpos, endpos, limit, clonereq.Url)
		clonereq.SetHeader("Range", fmt.Sprintf("bytes=%d-%d", startpos, endpos))
		err, rangeres := gae.requestEvent(nil, clonereq)
		//try again
		if nil != err {
			err, rangeres = gae.requestEvent(nil, clonereq)
		}
		if nil != err {
			log.Printf("Failed to fetch range chunk[%d:%d-%d] from %s for reason:%v", startpos, endpos, limit, clonereq.Url, err)
			gae.Close()
			break
		}
		rangeHttpRes, ok := rangeres.(*event.HTTPResponseEvent)
		if !ok {
			gae.Close()
			break
		}
		if rangeHttpRes.Status == 302 {
			location := rangeHttpRes.GetHeader("Location")
			xrange := rangeHttpRes.GetHeader("X-Range")
			if len(location) > 0 && len(xrange) > 0 {
				clonereq.Url = location
				clonereq.SetHeader("Range", xrange)
				err, rangeres = gae.requestEvent(nil, clonereq)
				if nil != err {
					err, rangeres = gae.requestEvent(nil, clonereq)
				}
				if nil != err {
					log.Printf("Failed to fetch range chunk[%s] from %s for reason:%v", xrange, clonereq.Url, err)
					break
				}
				rangeHttpRes, ok = rangeres.(*event.HTTPResponseEvent)
				if !ok {
					gae.Close()
					break
				}
			}
		}
		chunk := &rangeChunk{}
		chunk.start = startpos
		chunk.content = rangeHttpRes.Content.Bytes()
		gae.rangeFetchChannel <- chunk
		startpos = startpos + int(gae_cfg.FetchLimitSize*gae_cfg.ConcurrentRangeFetcher)
	}
	//end
	if !gae.closed {
		log.Printf("Session[%d]Fetch[%d] close successfully", req.GetHash(), index)
	} else {
		log.Printf("Session[%d]Fetch[%d] close abnormally", req.GetHash(), index)
	}
	gae.rangeFetchChannel <- &rangeChunk{start: -1}
}

func (gae *GAEHttpConnection) handleHttpRes(conn *SessionConnection, req *event.HTTPRequestEvent, ev *event.HTTPResponseEvent, rangeHeader string) error {
	httpres := ev.ToResponse()
	contentRange := ev.GetHeader("Content-Range")
	limit := 0
	if len(contentRange) > 0 {
		httpres.Header.Del("Content-Range")
		_, endpos, length := util.ParseContentRangeHeaderValue(contentRange)
		limit = length - 1
		if httpres.StatusCode < 300 {
			if len(rangeHeader) == 0 {
				httpres.StatusCode = 200
				httpres.Status = ""
				httpres.ContentLength = int64(length - gae.rangeStart)
			} else {
				start, end := util.ParseRangeHeaderValue(rangeHeader)
				if end == -1 {
					httpres.ContentLength = int64(length - start)
				} else {
					httpres.ContentLength = int64(end - start + 1)
					limit = end
				}
				httpres.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d\r\n", start, (int64(start)+httpres.ContentLength-1), length))
			}
		}

		if httpres.StatusCode >= 300 {
			httpres.Write(conn.LocalRawConn)
			return nil
		}
		if endpos < limit {
			if nil == gae.rangeFetchChannel {
				gae.rangeFetchChannel = make(chan *rangeChunk, 10)
			}
			for i := 0; i < int(gae_cfg.ConcurrentRangeFetcher); i++ {
				go gae.rangeFetch(req, i, endpos+1+i*int(gae_cfg.FetchLimitSize), limit)
			}
			//write response after launch goroutines
			httpres.Write(conn.LocalRawConn)
			responsedChunks := make(map[int]*rangeChunk)
			stopedWorker := uint32(0)
			gae.range_expected_pos = endpos + 1
			for {
				select {
				case chunk := <-gae.rangeFetchChannel:
					if nil != chunk {
						if chunk.start < 0 {
							stopedWorker = stopedWorker + 1
						} else {
							responsedChunks[chunk.start] = chunk
						}
					}
					for {
						if chunk, exist := responsedChunks[gae.range_expected_pos]; exist {
							_, err := conn.LocalRawConn.Write(chunk.content)
							delete(responsedChunks, gae.range_expected_pos)
							if nil != err {
								log.Printf("????????????????????????????%v\n", err)
								conn.LocalRawConn.Close()
								gae.Close()
								//return err
							} else {
								gae.range_expected_pos = gae.range_expected_pos + len(chunk.content)
							}

						} else {
							break
						}
					}

					if stopedWorker >= gae_cfg.ConcurrentRangeFetcher {
						if len(responsedChunks) > 0 {
							log.Printf("Session[%d]Rest %d unwrite chunks.\n", req.GetHash(), len(responsedChunks))
						}
						break
					}
				}
			}
		}
		return nil
	}
	err := httpres.Write(conn.LocalRawConn)
	return err
}

func (gae *GAEHttpConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	gae.closed = false
	if ev.GetType() == event.HTTP_REQUEST_EVENT_TYPE {
		httpreq := ev.(*event.HTTPRequestEvent)
		if strings.EqualFold(httpreq.Method, "CONNECT") {
			log.Printf("Session[%d]Request %s %s\n", httpreq.GetHash(), httpreq.Method, httpreq.RawReq.RequestURI)
			conn.LocalRawConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
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
				if hostPatternMatched(gae_cfg.InjectRange, httpreq.RawReq.Host) {
					httpreq.SetHeader("Range", "bytes=0-"+strconv.Itoa(int(gae_cfg.FetchLimitSize-1)))
				}
			}
			if strings.HasPrefix(httpreq.RawReq.RequestURI, "http://") {
				log.Printf("Session[%d]Request %s %s\n", httpreq.GetHash(), httpreq.Method, httpreq.RawReq.RequestURI)
			} else {
				log.Printf("Session[%d]Request %s %s%s\n", httpreq.GetHash(), httpreq.Method, httpreq.RawReq.Host, httpreq.RawReq.RequestURI)
			}
			//httpreq.SetHeader("Connection", "Close")
			httpreq.RemoveHeader("Proxy-Connection")
			err, res = gae.requestEvent(conn, ev)
			if nil != err {
				return
			}
			conn.State = STATE_RECV_HTTP
			if nil != conn {
				httpres := res.(*event.HTTPResponseEvent)
				if httpres.Status == 403 {
					log.Printf("ERROR:Session[%d]Request %s %s is forbidon\n", httpreq.GetHash(), httpreq.Method, httpreq.RawReq.Host)
					log.Printf("ERROR:Session[%d]Refer : %s\n", httpreq.GetHash(), httpreq.RawReq.Referer())
				}
				gae.handleHttpRes(conn, httpreq, httpres, rangeHeader)
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

//func (manager *GAE) GetArg() string {
//	return ""
//}

func initGAEConfig() {
	//init config
	gae_cfg = new(GAEConfig)
	if ua, exist := common.Cfg.GetProperty("GAE", "UserAgent"); exist {
		gae_cfg.UA = ua
	}
	gae_cfg.ConnectionMode = MODE_HTTP
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
	gae_cfg.ConcurrentRangeFetcher = 5
	if fetcher, exist := common.Cfg.GetIntProperty("GAE", "RangeConcurrentFetcher"); exist {
		gae_cfg.ConcurrentRangeFetcher = uint32(fetcher)
	}
	gae_cfg.FetchLimitSize = 256000
	if limit, exist := common.Cfg.GetIntProperty("GAE", "RangeFetchLimitSize"); exist {
		gae_cfg.FetchLimitSize = uint32(limit)
	}
	gae_cfg.RangeFetchRetryLimit = 1
	if limit, exist := common.Cfg.GetIntProperty("GAE", "RangeFetchRetryLimit"); exist {
		gae_cfg.RangeFetchRetryLimit = uint32(limit)
	}
	gae_cfg.InjectRange = []*regexp.Regexp{}
	if ranges, exist := common.Cfg.GetProperty("GAE", "InjectRange"); exist {
		gae_cfg.InjectRange = initHostMatchRegex(ranges)
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
	if enable, exist := common.Cfg.GetIntProperty("GAE", "Enable"); exist {
		gae_enable = (enable != 0)
		if enable == 0 {
			return errors.New("GAE not inited since [GAE] Enable=0")
		}
	}
	log.Println("Init GAE.")
	RegisteRemoteConnManager(manager)
	initGAEConfig()
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
	gae_use_shared_appid = false
	if index == 0 {
		gae_use_shared_appid = true
		err, appids := manager.fetchSharedAppIDs()
		if nil != err {
			return err
		}
		for _, appid := range appids {
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
			gae_enable = false
			return err
		}
		auth.token = conn.authToken
		conn.auth.token = conn.authToken
		manager.RecycleRemoteConnection(conn)
	}
	return nil
}

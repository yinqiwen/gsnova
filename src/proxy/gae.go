package proxy

import (
	"bufio"
	"bytes"
	"common"
	"crypto/tls"
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

var singleton_gae *GAE
var gae_cfg *GAEConfig
var gae_enable bool
var gae_use_shared_appid bool
var total_gae_conn_num uint32

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
			return fmt.Errorf("Invalid user/pass:%s", userpass)
		}
		auth.user = us[0]
		auth.passwd = us[1]
	} else {
		return fmt.Errorf("Invalid user/pass/appid: %s", line)
	}
	return nil
}

type GAEHttpConnection struct {
	auth               GAEAuth
	support_tunnel     bool
	over_tunnel        bool
	inject_range       bool
	authToken          string
	sess               *SessionConnection
	client             *http.Client
	manager            *GAE
	proxyURL           *url.URL
	tunnelChannel      chan event.Event
	tunnel_remote_addr string
	rangeStart         int
	range_expected_pos int
	closed             bool
}

func (conn *GAEHttpConnection) IsDisconnected() bool {
	return false
}

func (gae *GAEHttpConnection) Close() error {
	if gae.over_tunnel {
		gae.doCloseTunnel()
	}
	gae.closed = true
	return nil
}

func (conn *GAEHttpConnection) GetConnectionManager() RemoteConnectionManager {
	return conn.manager
}

func (conn *GAEHttpConnection) createHttpClient() *http.Client {
	client := new(http.Client)
	tlcfg := &tls.Config{}
	tlcfg.InsecureSkipVerify = true

	dial := func(n, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(n, addr, connTimeoutSecs)
		if err != nil {
			return nil, err
		}
		return conn, err
	}

	sslDial := func(n, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(n, addr, connTimeoutSecs)
		if err != nil {
			return nil, err
		}
		return tls.Client(conn, tlcfg), nil
	}
	proxyInfo, exist := common.Cfg.GetProperty("LocalProxy", "Proxy")
	if exist {

		if strings.HasPrefix(proxyInfo, "https") {
			dial = sslDial
		}
		tr := &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				if nil != conn.proxyURL {
					return conn.proxyURL, nil
				}
				proxy := getLocalUrlMapping(proxyInfo)
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
		client.Transport = tr
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
		client.Transport = tr
	}
	return client
}

func (conn *GAEHttpConnection) initHttpClient() {
	if nil != conn.client {
		return
	}
	conn.client = conn.createHttpClient()
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
		return fmt.Errorf("Invalid auth response.")
	} else {
		log.Printf("Auth token is %s\n", authres.Token)
		conn.authToken = authres.Token
		conn.support_tunnel = len(authres.Version) > 0
	}
	return nil
}

func (gae *GAEHttpConnection) requestEvent(client *http.Client, conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	auth := &gae.auth
	domain := auth.appid + ".appspot.com"
	if strings.Contains(auth.appid, ".") {
		domain = auth.appid
	}
	addr, _ := getLocalHostMapping(domain)
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
	if proxyInfo, exist := common.Cfg.GetProperty("LocalProxy", "Proxy"); exist {
		if strings.HasPrefix(proxyInfo, "http://") && strings.Contains(proxyInfo, "Google") {
			req.Method = string(CRLFs) + "POST"
		}
	}

	var response *http.Response
	response, err = client.Do(req)
	if nil != err {
		log.Printf("Failed to request data from GAE:%s for:%s\n", domain, err.Error())
		return err, nil
	} else {
		if response.StatusCode != 200 {
			log.Printf("Session[%d]Invalid response:%d\n", ev.GetHash(), response.StatusCode)
			return fmt.Errorf("Invalid response:%d", response.StatusCode), nil
		} else {
			buf := util.GetBuffer()
			n, err := io.Copy(buf, response.Body)
			if int64(n) < response.ContentLength {
				return fmt.Errorf("No sufficient space in body."), nil
			}
			if nil != err {
				return err, nil
			}
			response.Body.Close()
			if !tags.Decode(buf) {
				return fmt.Errorf("Failed to decode event tag"), nil
			}
			err, res = event.DecodeEvent(buf)
			if nil == err {
				res = event.ExtractEvent(res)
			}
			util.RecycleBuffer(buf)
			return err, res
		}
	}

	return nil, nil
}

func (gae *GAEHttpConnection) rangeFetch(req *event.HTTPRequestEvent, index, startpos, limit int, ch chan *rangeChunk) {
	clonereq := req.DeepClone()
	defer util.RecycleBuffer(clonereq.Content)
	client := gae.createHttpClient()
	for startpos < limit-1 && !gae.closed {
		if startpos-gae.range_expected_pos >= 2*1024*1024 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		endpos := startpos + int(gae_cfg.FetchLimitSize) - 1
		if endpos > limit {
			endpos = limit
		}
		log.Printf("Session[%d]Fetch[%d] range chunk[%d:%d-%d]from %s", req.GetHash(), index, startpos, endpos, limit, clonereq.Url)
		rangeHeader := fmt.Sprintf("bytes=%d-%d", startpos, endpos)
		clonereq.SetHeader("Range", rangeHeader)
		err, rangeres := gae.requestEvent(client, nil, clonereq)
		//try again
		if nil != err {
			err, rangeres = gae.requestEvent(client, nil, clonereq)
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
				clonereq.SetHeader("Range", rangeHeader)
				err, rangeres = gae.requestEvent(client, nil, clonereq)
				if nil != err {
					err, rangeres = gae.requestEvent(client, nil, clonereq)
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
		chunk.content = (rangeHttpRes.Content)
		//rangeHttpRes.Content.Reset()
		rangeHttpRes = nil
		ch <- chunk
		startpos = startpos + int(gae_cfg.FetchLimitSize*gae_cfg.ConcurrentRangeFetcher)
	}
	//end
	if !gae.closed {
		log.Printf("Session[%d]Fetch[%d] close successfully", req.GetHash(), index)
	} else {
		log.Printf("Session[%d]Fetch[%d] close abnormally", req.GetHash(), index)
	}
	ch <- &rangeChunk{start: -1}
}

func (gae *GAEHttpConnection) concurrentRrangeFetch(req *event.HTTPRequestEvent, httpres *http.Response, limit, endpos int) {
	rangeFetchChannel := make(chan *rangeChunk)
	for i := 0; i < int(gae_cfg.ConcurrentRangeFetcher); i++ {
		go gae.rangeFetch(req, i, endpos+1+i*int(gae_cfg.FetchLimitSize), limit, rangeFetchChannel)
	}
	//write response after launch goroutines
	httpres.Write(gae.sess.LocalRawConn)
	responsedChunks := make(map[int]*rangeChunk)
	stopedWorker := uint32(0)
	gae.range_expected_pos = endpos + 1

	try_write_chunk := func(chunk *rangeChunk) (bool, error) {
		if chunk.start == gae.range_expected_pos {
			_, err := gae.sess.LocalRawConn.Write(chunk.content.Bytes())
			if nil != err {
				log.Printf("????????????????????????????%v\n", err)
				gae.sess.LocalRawConn.Close()
				gae.Close()
			}
			gae.range_expected_pos = gae.range_expected_pos + chunk.content.Len()
			util.RecycleBuffer(chunk.content)
			return true, err
		}
		return false, nil
	}

	for {
		select {
		case chunk := <-rangeFetchChannel:
			if nil != chunk {
				if chunk.start < 0 {
					stopedWorker = stopedWorker + 1
				} else {
					if success, err := try_write_chunk(chunk); nil != err {
						break
					} else if !success {
						responsedChunks[chunk.start] = chunk
						log.Printf("Session[%d]recv chunk:%d, expected chunk:%d", req.GetHash(), chunk.start, gae.range_expected_pos)
					}
				}
				chunk = nil
			}
			for {
				if chunk, exist := responsedChunks[gae.range_expected_pos]; exist {
					_, err := gae.sess.LocalRawConn.Write(chunk.content.Bytes())
					delete(responsedChunks, chunk.start)
					util.RecycleBuffer(chunk.content)
					if nil != err {
						log.Printf("????????????????????????????%v\n", err)
						gae.sess.LocalRawConn.Close()
						gae.Close()
						//return err
					} else {
						gae.range_expected_pos = gae.range_expected_pos + chunk.content.Len()
					}
					chunk = nil
				} else {
					break
				}
			}
		}
		if stopedWorker >= gae_cfg.ConcurrentRangeFetcher {
			if len(responsedChunks) > 0 {
				log.Printf("Session[%d]Rest %d unwrite chunks.\n", req.GetHash(), len(responsedChunks))
			}
			break
		}
	}
	log.Printf("Session[%d]Exit concurrent range fetch.\n", req.GetHash())
}

func (gae *GAEHttpConnection) handleHttpRes(conn *SessionConnection, req *event.HTTPRequestEvent, ev *event.HTTPResponseEvent, rangeHeader string) (*http.Response, error) {
	httpres := ev.ToResponse()
	contentRange := ev.GetHeader("Content-Range")
	limit := 0
	if len(contentRange) > 0 {
		httpres.Header.Del("Content-Range")
		startpos, endpos, length := util.ParseContentRangeHeaderValue(contentRange)
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
			return httpres, nil
		}
		if endpos < limit {
			if gae_cfg.ConcurrentRangeFetcher > 1 {
				gae.concurrentRrangeFetch(req, httpres, limit, endpos)
			} else {
				httpres.Write(conn.LocalRawConn)
				for endpos < length-1 {
					startpos = endpos + 1
					endpos = endpos + int(gae_cfg.FetchLimitSize)
					if endpos >= length-1 {
						endpos = length - 1
					}
					rangeHeader := fmt.Sprintf("bytes=%d-%d", startpos, endpos)
					req.SetHeader("Range", rangeHeader)
					err, rangeres := gae.requestEvent(gae.client, nil, req)
					//try again
					if nil != err {
						err, rangeres = gae.requestEvent(gae.client, nil, req)
					}
					if nil != err {
						return httpres, err
					}
					rangeHttpRes := rangeres.(*event.HTTPResponseEvent)
					if rangeHttpRes.Status == 302 {
						location := rangeHttpRes.GetHeader("Location")
						xrange := rangeHttpRes.GetHeader("X-Range")
						log.Printf("########Session[%d]X-Range=%s, Location= %s\n", req.GetHash(), xrange, location)
						if len(location) > 0 && len(xrange) > 0 {
							req.Url = location
							req.SetHeader("Range", rangeHeader)
							err, rangeres = gae.requestEvent(gae.client, nil, req)
							if nil != err {
								err, rangeres = gae.requestEvent(gae.client, nil, req)
							}
							if nil != err {
								return httpres, err
							}
						}
					}
					_, err = conn.LocalRawConn.Write(rangeres.(*event.HTTPResponseEvent).Content.Bytes())
					util.RecycleBuffer(rangeres.(*event.HTTPResponseEvent).Content)
					if nil != err {
						return httpres, err
					}
				}
			}
		}
		return httpres, nil
	}
	err := httpres.Write(conn.LocalRawConn)
	return httpres, err
}

func (gae *GAEHttpConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	gae.closed = false
	gae.sess = conn
	if gae.over_tunnel {
		return gae.requestOverTunnel(conn, ev)
	}
	gae.initHttpClient()
	if ev.GetType() == event.HTTP_REQUEST_EVENT_TYPE {
		httpreq := ev.(*event.HTTPRequestEvent)
		if strings.EqualFold(httpreq.Method, "CONNECT") {
			//try https over tunnel
			if gae.support_tunnel {
				gae.over_tunnel = true
				return gae.requestOverTunnel(conn, ev)
			}
			log.Printf("Session[%d]Request %s\n", httpreq.GetHash(), util.GetURLString(httpreq.RawReq, true))
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
				if hostPatternMatched(gae_cfg.InjectRange, httpreq.RawReq.Host) || gae.inject_range {
					httpreq.SetHeader("Range", "bytes=0-"+strconv.Itoa(int(gae_cfg.FetchLimitSize-1)))
				}
			}
			log.Printf("Session[%d]Request %s\n", httpreq.GetHash(), util.GetURLString(httpreq.RawReq, true))
			//httpreq.SetHeader("Connection", "Close")
			httpreq.RemoveHeader("Proxy-Connection")
			err, res = gae.requestEvent(gae.client, conn, ev)
			if nil != err {
				return
			}
			conn.State = STATE_RECV_HTTP
			httpresev := res.(*event.HTTPResponseEvent)
			if httpresev.Status == 403 {
				log.Printf("ERROR:Session[%d]Request %s %s is forbidon\n", httpreq.GetHash(), httpreq.Method, httpreq.RawReq.Host)
			}
			httpres, err := gae.handleHttpRes(conn, httpreq, httpresev, rangeHeader)
			//if !httpres.IsKeepAlive(){
			if nil != err || !util.IsResponseKeepAlive(httpres) || !util.IsRequestKeepAlive(httpreq.RawReq) {
				conn.LocalRawConn.Close()
				conn.State = STATE_SESSION_CLOSE
			}

			return err, nil
		}
	}
	return gae.requestEvent(gae.client, conn, ev)
}

type GAE struct {
	auths      *util.ListSelector
	idle_conns chan RemoteConnection
}

func (manager *GAE) shareAppId(appid, email string, operation uint32) error {
	var ev event.ShareAppIDEvent
	ev.AppId = appid
	ev.Email = email
	ev.Operation = operation
	var auth GAEAuth
	auth.appid = gae_cfg.MasterAppID
	auth.user = ANONYMOUSE
	auth.passwd = ANONYMOUSE
	conn := new(GAEHttpConnection)
	conn.auth = auth
	conn.manager = manager
	err, res := conn.Request(nil, &ev)
	if nil != err {
		return err
	}
	admin_res := res.(*event.AdminResponseEvent)
	if len(admin_res.ErrorCause) > 0 {
		return fmt.Errorf("%s", admin_res.ErrorCause)
	}
	return nil
}

func (manager *GAE) RecycleRemoteConnection(conn RemoteConnection) {
	select {
	case manager.idle_conns <- conn:
		// Buffer on free list; nothing more to do.
	default:
		// Free list full, just carry on.
	}
	total_gae_conn_num = total_gae_conn_num - 1
	conn.(*GAEHttpConnection).inject_range = false
}

func (manager *GAE) GetRemoteConnection(ev event.Event, attrs map[string]string) (RemoteConnection, error) {
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
	if containsAttr(attrs, ATTR_TUNNEL) {
		b.(*GAEHttpConnection).over_tunnel = true && b.(*GAEHttpConnection).support_tunnel
	} else {
		b.(*GAEHttpConnection).over_tunnel = false
	}
	if containsAttr(attrs, ATTR_RANGE) {
		b.(*GAEHttpConnection).inject_range = true
	}
	total_gae_conn_num = total_gae_conn_num + 1
	return b, nil
}

func (manager *GAE) GetName() string {
	return GAE_NAME
}

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
	return fmt.Errorf("Invalid response for shared appid."), nil
}

func (manager *GAE) Init() error {
	if enable, exist := common.Cfg.GetIntProperty("GAE", "Enable"); exist {
		gae_enable = (enable != 0)
		if enable == 0 {
			return fmt.Errorf("GAE not inited since [GAE] Enable=0")
		}
	}
	log.Println("Init GAE.")
	singleton_gae = manager
	RegisteRemoteConnManager(manager)
	initGAEConfig()
	manager.idle_conns = make(chan RemoteConnection, gae_cfg.ConnectionPoolSize)
	manager.auths = new(util.ListSelector)
	authArray := make([]*GAEAuth, 0)
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
		//manager.auths.Add(&auth)
		authArray = append(authArray, &auth)
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
			authArray = append(authArray, &auth)
			//manager.auths.Add(&auth)
		}
	}
	for _, auth := range authArray {
		conn := new(GAEHttpConnection)

		conn.auth = *auth
		conn.manager = manager
		err := conn.Auth()
		if nil != err {
			conn.Close()
			//try again
			log.Printf("Failed first to auth appid:%s\n", err.Error())
			err = conn.Auth()
		}
		if nil != err {
			log.Printf("Failed to auth appid:%s\n", err.Error())
			continue
		}
		auth.token = conn.authToken
		conn.auth.token = conn.authToken
		total_gae_conn_num = total_gae_conn_num + 1
		manager.RecycleRemoteConnection(conn)
		manager.auths.Add(auth)
	}
	if manager.auths.Size() == 0 {
		gae_enable = false
		return fmt.Errorf("[ERROR]No valid appid found.")
	}
	return nil
}

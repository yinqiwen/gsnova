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
	Proxy                  string
}

var singleton_gae *GAE
var gae_cfg *GAEConfig
var GAEEnable bool
var gae_use_shared_appid bool
var total_gae_conn_num uint32

var gaeHttpClient *http.Client

func initGAEClient() {
	if nil != gaeHttpClient {
		return
	}
	client := new(http.Client)
	tlcfg := &tls.Config{}
	tlcfg.InsecureSkipVerify = true

	dial := func(n, addr string) (net.Conn, error) {
		remote := getAddressMapping(addr)
		conn, err := net.DialTimeout(n, remote, connTimeoutSecs)
		if err != nil {
			expireBlockVerifyCache(addr)
			//try again
			remote = getAddressMapping(addr)
			conn, err = net.DialTimeout(n, remote, connTimeoutSecs)
		}
		if err != nil {
			expireBlockVerifyCache(addr)
			return nil, err
		}
		return conn, err
	}

	sslDial := func(n, addr string) (net.Conn, error) {
		var remote string
		var err error
		var conn net.Conn
		for i := 0; i < 3; i++ {
			remote = getAddressMapping(addr)
			conn, err = net.DialTimeout(n, remote, connTimeoutSecs)
			if err != nil {
				expireBlockVerifyCache(addr)
			}
		}
		if err != nil {
			return nil, err
		}
		return tls.Client(conn, tlcfg), nil
	}

	if len(gae_cfg.Proxy) > 0 {
		if strings.Contains(gae_cfg.Proxy, "Google") {
			if strings.HasPrefix(gae_cfg.Proxy, "https://") {
				dial = sslDial
			}
		} else {
			dial = net.Dial
		}
		tr := &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse(gae_cfg.Proxy)
			},
			Dial:                dial,
			TLSClientConfig:     tlcfg,
			DisableCompression:  true,
			MaxIdleConnsPerHost: int(gae_cfg.ConnectionPoolSize),
		}
		client.Transport = tr
	} else {
		if mode, exist := common.Cfg.GetProperty("GAE", "ConnectionMode"); exist {
			if strings.EqualFold(mode, MODE_HTTPS) {
				dial = sslDial
			}
		}
		tr := &http.Transport{
			Dial:                dial,
			TLSClientConfig:     tlcfg,
			DisableCompression:  true,
			MaxIdleConnsPerHost: int(gae_cfg.ConnectionPoolSize),
		}
		client.Transport = tr
	}
	gaeHttpClient = client
}

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
	//auth               GAEAuth
	gaeAuth            *GAEAuth
	support_tunnel     bool
	over_tunnel        bool
	inject_range       bool
	//authToken          string
	sess               *SessionConnection
	manager            *GAE
	tunnelChannel      chan event.Event
	tunnel_remote_addr string
	closed             bool
	rangeWorker        *rangeFetchTask
}

func (gae *GAEHttpConnection) Close() error {
	if gae.over_tunnel {
		gae.doCloseTunnel()
	}
	if nil != gae.rangeWorker {
		gae.rangeWorker.Close()
	}
	gae.closed = true
	gae.manager.RecycleRemoteConnection(gae)
	return nil
}

func (conn *GAEHttpConnection) GetConnectionManager() RemoteConnectionManager {
	return conn.manager
}

func (conn *GAEHttpConnection) Auth(auth *GAEAuth) error {
	var authEvent event.AuthRequestEvent
	authEvent.User = auth.user
	authEvent.Passwd = auth.passwd
	authEvent.Appid = auth.appid
	conn.gaeAuth = auth
	err, res := conn.Request(nil, &authEvent)
	if nil != err {
		log.Println(err)
		return err
	}
	if authres, ok := res.(*event.AuthResponseEvent); !ok {
		log.Printf("Type is  %d\n", res.GetType())
		return fmt.Errorf("Invalid auth response.")
	} else {
		if len(authres.Token) == 0 {
			return fmt.Errorf("%s", authres.Error)
		}
		log.Printf("Auth token is %s\n", authres.Token)
		auth.token = authres.Token
		conn.support_tunnel = len(authres.Version) > 0
	}
	return nil
}

func (gae *GAEHttpConnection) requestEvent(client *http.Client, conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	auth := gae.gaeAuth
	if nil == auth {
		auth = gae.manager.auths.Select().(*GAEAuth)
	}
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
	response, err = gaeHttpClient.Do(req)
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

func (gae *GAEHttpConnection) doRangeFetch(req *http.Request, firstChunkRes *http.Response) {
	task := new(rangeFetchTask)
	task.FetchLimit = int(gae_cfg.FetchLimitSize)
	task.FetchWorkerNum = int(gae_cfg.ConcurrentRangeFetcher)
	task.SessionID = gae.sess.SessionID
	//	task.TaskValidation = func() bool {
	//		return !util.IsDeadConnection(gae.sess.LocalRawConn)
	//	}
	gae.rangeWorker = task
	fetch := func(preq *http.Request) (*http.Response, error) {
		ev := new(event.HTTPRequestEvent)
		ev.FromRequest(preq)
		ev.SetHash(gae.sess.SessionID)
		err, xres := gae.requestEvent(gaeHttpClient, gae.sess, ev)
		if nil != err {
			//try again
			err, xres = gae.requestEvent(gaeHttpClient, gae.sess, ev)
		}
		if nil == err {
			httpresev := xres.(*event.HTTPResponseEvent)
			httpres := httpresev.ToResponse()
			return httpres, err
		}
		return nil, err
	}
	pres, err := task.SyncGet(req, firstChunkRes, fetch)
	if nil == err {
		err = pres.Write(gae.sess.LocalRawConn)
		if nil != err {
			task.Close()
		}
		if nil != pres.Body {
			pres.Body.Close()
		}
	}
	if nil != err {
		log.Printf("Session[%d]Range task failed for reason:%v\n", gae.sess.SessionID, err)
	}
	if nil != err || !util.IsResponseKeepAlive(pres) || !util.IsRequestKeepAlive(req) {
		gae.sess.LocalRawConn.Close()
		gae.sess.State = STATE_SESSION_CLOSE
	}
}

func (gae *GAEHttpConnection) handleHttpRes(conn *SessionConnection, req *event.HTTPRequestEvent, ev *event.HTTPResponseEvent) (*http.Response, error) {
	originRange := req.RawReq.Header.Get("Range")
	contentRange := ev.GetHeader("Content-Range")
	if ev.Status == 206 && len(contentRange) > 0 && strings.EqualFold(req.Method, "GET") {
		_, end, length := util.ParseContentRangeHeaderValue(contentRange)
		if len(originRange) > 0 {
			_, oe := util.ParseRangeHeaderValue(originRange)
			if oe > 0 {
				length = oe + 1
			}
		}
		if length > end+1 {
			gae.doRangeFetch(req.RawReq, ev.ToResponse())
			return nil, nil
		}
		if len(originRange) == 0 {
			ev.Status = 200
			ev.RemoveHeader("Content-Range")
		}
	}
	httpres := ev.ToResponse()
	err := httpres.Write(conn.LocalRawConn)
	return httpres, err
}

func (gae *GAEHttpConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	gae.closed = false
	gae.sess = conn
	if gae.over_tunnel {
		return gae.requestOverTunnel(conn, ev)
	}
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

			log.Printf("Session[%d]Request %s\n", httpreq.GetHash(), util.GetURLString(httpreq.RawReq, true))
			var httpres *http.Response
			if strings.EqualFold(httpreq.Method, "GET") {
				if hostPatternMatched(gae_cfg.InjectRange, httpreq.RawReq.Host) || gae.inject_range {
					//conn.State = STATE_RECV_HTTP
					gae.doRangeFetch(httpreq.RawReq, nil)
					return nil, nil
				}
			}
			err, res = gae.requestEvent(gaeHttpClient, conn, ev)
			if nil != err {
				//try again
				err, res = gae.requestEvent(gaeHttpClient, conn, ev)
			}
			if nil != err || nil == res {
				return
			}
			conn.State = STATE_RECV_HTTP
			httpresev := res.(*event.HTTPResponseEvent)
			if httpresev.Status == 403 {
				log.Printf("ERROR:Session[%d]Request %s %s is forbidon\n", httpreq.GetHash(), httpreq.Method, httpreq.RawReq.Host)
			}
			httpres, err = gae.handleHttpRes(conn, httpreq, httpresev)
			//			if nil != httpres {
			//				log.Printf("Session[%d]Response %d %v\n", httpreq.GetHash(), httpres.StatusCode, httpres.Header)
			//			}
			if nil != err || !util.IsResponseKeepAlive(httpres) || !util.IsRequestKeepAlive(httpreq.RawReq) {
				conn.LocalRawConn.Close()
				conn.State = STATE_SESSION_CLOSE
			}
			return err, nil
		}
	}
	return gae.requestEvent(gaeHttpClient, conn, ev)
}

type GAE struct {
	auths *util.ListSelector
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
	//conn.auth = auth
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
	total_gae_conn_num = total_gae_conn_num - 1

}

func (manager *GAE) GetRemoteConnection(ev event.Event, attrs map[string]string) (RemoteConnection, error) {
	if !GAEEnable {
		return nil, fmt.Errorf("No GAE connection available.")
	}
	gae := new(GAEHttpConnection)
	//gae.authToken = gae.auth.token
	gae.manager = manager

	if containsAttr(attrs, ATTR_TUNNEL) {
		gae.over_tunnel = true && gae.support_tunnel
	} else {
		gae.over_tunnel = false
	}
	if containsAttr(attrs, ATTR_RANGE) {
		gae.inject_range = true
	}
	//found := false
	if containsAttr(attrs, ATTR_APP) {
		appid := attrs[ATTR_APP]
		for _, tmp := range manager.auths.ArrayValues() {
			auth := tmp.(*GAEAuth)
			if auth.appid == appid {
				gae.gaeAuth = auth
				//found = true
				break
			}
		}
	}
	//	if !found {
	//		gae.auth = *(manager.auths.Select().(*GAEAuth))
	//	}

	total_gae_conn_num = total_gae_conn_num + 1
	return gae, nil
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
	if proxy, exist := common.Cfg.GetProperty("GAE", "Proxy"); exist {
		gae_cfg.Proxy = proxy
	}

}

func (manager *GAE) fetchSharedAppIDs() (error, []string) {
	var auth GAEAuth
	auth.appid = gae_cfg.MasterAppID
	auth.user = ANONYMOUSE
	auth.passwd = ANONYMOUSE
	conn := new(GAEHttpConnection)
	conn.gaeAuth = &auth
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
		GAEEnable = (enable != 0)
		if enable == 0 {
			return fmt.Errorf("GAE not inited since [GAE] Enable=0")
		}
	}
	log.Println("Init GAE.")
	singleton_gae = manager
	RegisteRemoteConnManager(manager)
	initGAEConfig()
	initGAEClient()
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
		authArray = append(authArray, &auth)
		index = index + 1
	}
	//no appid found, fetch shared from master
	gae_use_shared_appid = false
	if index == 0 && (!C4Enable && !SSHEnable) {
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
		}
	}
	for _, auth := range authArray {
		conn := new(GAEHttpConnection)
		//conn.auth = *auth
		conn.manager = manager
		err := conn.Auth(auth)
		if nil != err {
			conn.Close()
			//try again
			log.Printf("Failed first to auth appid:%s\n", err.Error())
			err = conn.Auth(auth)
		}
		if nil != err {
			log.Printf("Failed to auth appid:%s\n", err.Error())
			continue
		}
		//auth.token = conn.authToken
		//conn.auth.token = conn.authToken
		total_gae_conn_num = total_gae_conn_num + 1
		manager.auths.Add(auth)
	}
	if manager.auths.Size() == 0 {
		GAEEnable = false
		return fmt.Errorf("No valid appid found.")
	}
	return nil
}

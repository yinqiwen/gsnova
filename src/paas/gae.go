package paas

import (
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
	Compressor     uint32
	Encrypter      uint32
	InjectRange    []string
	UA             string
	ConnectionMode string
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
	auth      GAEAuth
	authToken string
	client    *http.Client
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
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/octet-stream")
	//log.Println(req)
	if response, err := gae.client.Do(req); nil != err {
		log.Printf("Failed to request data from GAE:%s\n", err.Error())
		return err, nil
	} else {
		if response.StatusCode != 200 {
			log.Printf("Invalid response:%d\n", response.StatusCode)
			return errors.New("Invalid response"), nil
		} else {
			log.Printf("Response with content len:%d\n", response.ContentLength)
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
			return err, res
		}
	}
	return nil, nil
}

func (gae *GAEHttpConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	if ev.GetType() == event.HTTP_REQUEST_EVENT_TYPE {
		httpreq := ev.(*event.HTTPRequestEvent)
		if strings.EqualFold(httpreq.Method, "CONNECT") {
			conn.LocalRawConn.Write([]byte("200 OK\r\n\r\n"))
			conn.LocalRawConn = tls.Server(conn.LocalRawConn, nil)
			conn.State = STATE_RECV_HTTP
			return nil, nil
		} else {

		}
	}
	return gae.requestEvent(conn, ev)
	//	err, res := gae.requestEvent(conn, ev)
	//	if nil != err {
	//	}
}

type GAE struct {
	auths      *util.ListSelector
	idle_conns chan *GAEHttpConnection
}

func (manager *GAE) RecycleRemoteConnection(conn *GAEHttpConnection) {
	select {
	case manager.idle_conns <- conn:
		// Buffer on free list; nothing more to do.
	default:
		// Free list full, just carry on.
	}
}

func (manager *GAE) GetRemoteConnection(ev event.Event) (RemoteConnection, error) {
	var b *GAEHttpConnection
	// Grab a buffer if available; allocate if not.
	select {
	case b = <-manager.idle_conns:
		// Got one; nothing more to do.
	default:
		// None free, so allocate a new one.
		b = new(GAEHttpConnection)
		b.authToken = manager.auths.Select().(*GAEAuth).token
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
}

func (manager *GAE) Init() error {
	log.Println("Init GAE.")
	RegisteRemoteConnManager(manager)
	initConfig()
	manager.idle_conns = make(chan *GAEHttpConnection, 20)
	manager.auths = new(util.ListSelector)
	index := 0
	for {
		v, exist := common.Cfg.GetProperty("GAE", "WorkerNode["+strconv.Itoa(index)+"]")
		if !exist {
			break
		}
		var auth GAEAuth
		if err := auth.parse(v); nil != err {
			return err
		}
		conn := new(GAEHttpConnection)
		conn.auth = auth
		if err := conn.Auth(); nil != err {
			log.Printf("Failed to auth appid:%s\n", err.Error())
			return err
		}
		auth.token = conn.authToken
		manager.auths.Add(&auth)
		manager.RecycleRemoteConnection(conn)
		index = index + 1
	}
	return nil
}

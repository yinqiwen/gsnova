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
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"util"
)

const (
	ANONYMOUSE = "anonymouse"
)

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

func (conn *GAEHttpConnection) Auth() error {
	conn.client = new(http.Client)
	proxyInfo, exist := common.Cfg.GetProperty("LocalProxy", "Proxy")
	if exist {
		log.Printf("GAE use proxy:%s\n", proxyInfo)
		tlcfg := &tls.Config{}
		tlcfg.InsecureSkipVerify = true
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
			TLSClientConfig:    tlcfg,
			DisableCompression: true,
		}
		conn.client.Transport = tr
	}
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

func (gae *GAEHttpConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	domain := gae.auth.appid + ".appspot.com"
	addr := util.GetHost(domain)
	scheme := "http"
	mode, exist := common.Cfg.GetProperty("GAE", "ConnectionMode")
	if exist {
		if strings.EqualFold(mode, "https") {
			scheme = "https"
		}
	}
	var buf bytes.Buffer
	var tags event.EventHeaderTags
	tags.Token = gae.authToken
	tags.Encode(&buf)
	if ev.GetType() == event.HTTP_REQUEST_EVENT_TYPE {
		var compress event.CompressEvent
		compress.SetHash(ev.GetHash())
		compress.Ev = ev
		compress.CompressType = event.COMPRESSOR_SNAPPY
		var encrypt event.EncryptEvent
		encrypt.SetHash(ev.GetHash())
		encrypt.EncryptType = event.ENCRYPTER_SE1
		encrypt.Ev = &compress
		event.EncodeEvent(&buf, &encrypt)
	} else {
		event.EncodeEvent(&buf, ev)
	}
	req := &http.Request{
		Method:        "POST",
		URL:           &url.URL{Scheme: scheme, Host: addr, Path: "/invoke"},
		Host:          addr,
		Header:        make(http.Header),
		Body:          ioutil.NopCloser(&buf),
		ContentLength: int64(buf.Len()),
	}

	if ua, exist := common.Cfg.GetProperty("GAE", "UserAgent"); exist {
		req.Header.Set("User-Agent", ua)
	}
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/octet-stream")
	log.Println(req)
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

type GAE struct {
	conns map[string]*util.ListSelector
	auths map[string]GAEAuth
}

func (manager *GAE) GetRemoteConnection(ev event.Event) (RemoteConnection, error) {

	return nil, nil
}

func (manager *GAE) GetName() string {
	return GAE_NAME
}

func (manager *GAE) Init() error {
	log.Println("Init GAE.")
	RegisteRemoteConnManager(manager)
	manager.conns = make(map[string]*util.ListSelector)
	manager.auths = make(map[string]GAEAuth)
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
		manager.auths[auth.appid] = auth
		manager.conns[auth.appid] = &(util.ListSelector{})
		manager.conns[auth.appid].Add(conn)
		index = index + 1
	}
	return nil
}

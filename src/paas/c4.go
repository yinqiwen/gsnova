package paas

import (
	"bytes"
	"common"
	"container/list"
	"errors"
	"event"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"util"
)

type C4Config struct {
	Compressor uint32
	Encrypter  uint32
	UA         string
}

var c4_cfg *C4Config

type C4HttpAssistConnection struct {
	server  string
	manager *C4
	ev_list *list.List
	client  *http.Client
}

type C4HttpConnection struct {
	server  string
	manager *C4
	ev_list *list.List
	client  *http.Client
}

func (c4 *C4HttpConnection) Auth() error {
	c4.client = new(http.Client)
	return nil
}

func (c4 *C4HttpConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	if nil != ev {
		var compress event.CompressEventV2
		compress.SetHash(ev.GetHash())
		compress.Ev = ev
		compress.CompressType = c4_cfg.Compressor
		var encrypt event.EncryptEventV2
		encrypt.SetHash(ev.GetHash())
		encrypt.EncryptType = c4_cfg.Encrypter
		encrypt.Ev = &compress
		c4.ev_list.PushBack(ev)
	}
	var buf bytes.Buffer

	req := &http.Request{
		Method:        "POST",
		URL:           &url.URL{Scheme: "http", Host: c4.server, Path: "/invoke"},
		Host:          c4.server,
		Header:        make(http.Header),
		Body:          ioutil.NopCloser(&buf),
		ContentLength: int64(buf.Len()),
	}
	if len(gae_cfg.UA) > 0 {
		req.Header.Set("User-Agent", c4_cfg.UA)
	}
	req.Close = false
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/octet-stream")
	//log.Println(gae.auth)
	if response, err := c4.client.Do(req); nil != err {
		log.Printf("Failed to request data from GAE:%s\n", err.Error())
		return err, nil
	} else {
		if response.StatusCode != 200 {
			log.Printf("Invalid response:%d\n", response.StatusCode)
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
			//trigger EOF to recycle idle conn in net.http
			tmp := make([]byte, 1)
			response.Body.Read(tmp)
		}
	}
	return nil, nil
}
func (c4 *C4HttpConnection) GetConnectionManager() RemoteConnectionManager {
	return nil
}

type C4RSocketConnection struct {
}

type C4 struct {
	auths *util.ListSelector
	conns map[string]RemoteConnection
}

func (manager *C4) GetRemoteConnection(ev event.Event) (RemoteConnection, error) {
	if len(manager.conns) == 0 {
		return nil, errors.New("No available C4 connection")
	}
	server := manager.auths.Select().(string)
	conn, exist := manager.conns[server]
	if !exist {
		return nil, errors.New("No available C4 connection")
	}
	return conn, nil
}
func (manager *C4) RecycleRemoteConnection(conn RemoteConnection) {

}
func (manager *C4) GetName() string {
	return C4_NAME
}

func initC4Config() {
	//init config
	c4_cfg = new(C4Config)
	if ua, exist := common.Cfg.GetProperty("C4", "UserAgent"); exist {
		c4_cfg.UA = ua
	}
	//	gae_cfg.ConnectionMode = "HTTP"
	//	if cm, exist := common.Cfg.GetProperty("GAE", "ConnectionMode"); exist {
	//		gae_cfg.ConnectionMode = cm
	//	}
	c4_cfg.Compressor = event.COMPRESSOR_SNAPPY
	if compress, exist := common.Cfg.GetProperty("GAE", "Compressor"); exist {
		if strings.EqualFold(compress, "None") {
			c4_cfg.Compressor = event.COMPRESSOR_NONE
		}
	}
	c4_cfg.Encrypter = event.ENCRYPTER_SE1
	if compress, exist := common.Cfg.GetProperty("GAE", "Encrypter"); exist {
		if strings.EqualFold(compress, "None") {
			c4_cfg.Compressor = event.ENCRYPTER_NONE
		}
	}
	//	gae_cfg.ConnectionPoolSize = 20
	//	if poosize, exist := common.Cfg.GetIntProperty("GAE", "ConnectionPoolSize"); exist {
	//		gae_cfg.ConnectionPoolSize = uint32(poosize)
	//	}
}

func (manager *C4) Init() error {
	log.Println("Init C4.")
	initC4Config()
	manager.conns = make(map[string]RemoteConnection)
	manager.auths = new(util.ListSelector)
	index := 0
	for {
		v, exist := common.Cfg.GetProperty("C4", "WorkerNode["+strconv.Itoa(index)+"]")
		if !exist || len(v) == 0 {
			break
		}
		manager.auths.Add(v)
		index = index + 1
	}
	//no appid found, fetch shared from master
	if index == 0 {
		return errors.New("No configed C4 server.")
	}
	for _, au := range manager.auths.ArrayValues() {
		conn := new(C4HttpConnection)
		conn.server = au.(string)
		conn.manager = manager
		if err := conn.Auth(); nil != err {
			log.Printf("Failed to auth server:%s\n", err.Error())
			return err
		}
		manager.conns[conn.server] = conn
	}
	return nil
}

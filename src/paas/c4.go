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
	"time"
	"util"
)

type C4Config struct {
	Compressor         uint32
	Encrypter          uint32
	UA                 string
	ConnectionPoolSize uint32
	MinWritePeriod     uint32
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
	//ev_list *list.List
	client      []*http.Client
	assit       *http.Client
	ev_chan     []chan event.Event
	local_conns []map[int32]*SessionConnection
}

func (c4 *C4HttpConnection) Auth() error {
	c4.assit = &(http.Client{})
	c4.client = make([]*http.Client, c4_cfg.ConnectionPoolSize)
	c4.ev_chan = make([]chan event.Event, c4_cfg.ConnectionPoolSize)
	c4.local_conns = make([]map[int32]*SessionConnection, c4_cfg.ConnectionPoolSize)
	for i := 0; i < int(c4_cfg.ConnectionPoolSize); i++ {
		c4.client[i] = &(http.Client{})
		c4.ev_chan[i] = make(chan event.Event, 1024)
		c4.local_conns[i] = make(map[int32]*SessionConnection)
		go c4.proces(i)
	}
	go c4.assistLoop()
	return nil
}

func (c4 *C4HttpConnection) assistLoop() {
	var empty bytes.Buffer
	for {
		c4.processClient(c4.assit, &empty)
        time.Sleep(time.Duration(c4_cfg.MinWritePeriod) * time.Millisecond)
	}
}

func (c4 *C4HttpConnection) processRecvEvent(ev event.Event) error {
   
   return nil
}

func (c4 *C4HttpConnection) processClient(cli *http.Client, buf *bytes.Buffer) error {
	req := &http.Request{
		Method:        "POST",
		URL:           &url.URL{Scheme: "http", Host: c4.server, Path: "/invoke"},
		Host:          c4.server,
		Header:        make(http.Header),
		Body:          ioutil.NopCloser(buf),
		ContentLength: int64(buf.Len()),
	}
	if len(gae_cfg.UA) > 0 {
		req.Header.Set("User-Agent", c4_cfg.UA)
	}
	req.Close = false
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/octet-stream")
	if buf.Len() == 0 {
		req.Header.Set("ClientActor", "Assist")
	} else {
		req.Header.Set("ClientActor", "Primary")
	}
	req.Header.Set("MaxResponseSize", strconv.Itoa(512*1024))
	if response, err := cli.Do(req); nil != err {
		log.Printf("Failed to request data from C4:%s\n", err.Error())
		return err
	} else {
		if response.StatusCode != 200 {
			log.Printf("Invalid response:%d\n", response.StatusCode)
			return errors.New("Invalid response")
		} else {
			//log.Printf("Response with content len:%d\n", response.ContentLength)
			content := make([]byte, response.ContentLength)
			n, err := io.ReadFull(response.Body, content)
			if int64(n) < response.ContentLength {
				return errors.New("No sufficient space in body.")
			}
			if nil != err {
				return err
			}
			//trigger EOF to recycle idle conn in net.http
			tmp := make([]byte, 1)
			response.Body.Read(tmp)
			buf := bytes.NewBuffer(content[0:response.ContentLength])
			for {
				if buf.Len() == 0 {
					break
				}
				err, res := event.DecodeEvent(buf)
				if nil == err {
					res = event.ExtractEvent(res)
					c4.processRecvEvent(res)
				} else {
					return err
				}
			}
		}
	}
	return nil
}

func (c4 *C4HttpConnection) proces(index int) error {
	client := c4.client[index]
	ev_chan := c4.ev_chan[index]
	//conn_table := c4.local_conns[index]
	for {
		var buf bytes.Buffer
		for {
			select {
			case ev := <-ev_chan:
				event.EncodeEvent(&buf, ev)
			default:
				break
			}
		}
		if buf.Len() > 0 {
			c4.processClient(client, &buf)
		} else {
			time.Sleep(time.Duration(c4_cfg.MinWritePeriod) * time.Millisecond)
		}
	}
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
		index := ev.GetHash() % int32(c4_cfg.ConnectionPoolSize)
		c4.ev_chan[index] <- &encrypt
		c4.local_conns[index][ev.GetHash()] = conn
		conn.State = STATE_RECV_HTTP_CHUNK
	}
	return nil, nil
}
func (c4 *C4HttpConnection) GetConnectionManager() RemoteConnectionManager {
	return c4.manager
}

type C4RSocketConnection struct {
}

type C4 struct {
	auths      *util.ListSelector
	idle_conns chan RemoteConnection
}

func (manager *C4) RecycleRemoteConnection(conn RemoteConnection) {
	select {
	case manager.idle_conns <- conn:
		// Buffer on free list; nothing more to do.
	default:
		// Free list full, just carry on.
	}
}

func (manager *C4) GetRemoteConnection(ev event.Event) (RemoteConnection, error) {
	var b RemoteConnection
	// Grab a buffer if available; allocate if not.
	select {
	case b = <-manager.idle_conns:
		// Got one; nothing more to do.
	default:
		// None free, so allocate a new one.
		c4 := new(C4HttpConnection)
		c4.manager = manager
		c4.Auth()
		b = c4
		//b.auth = 
	} // Read next message from the net.
	return b, nil
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
	if compress, exist := common.Cfg.GetProperty("C4", "Compressor"); exist {
		if strings.EqualFold(compress, "None") {
			c4_cfg.Compressor = event.COMPRESSOR_NONE
		}
	}
	c4_cfg.Encrypter = event.ENCRYPTER_SE1
	if compress, exist := common.Cfg.GetProperty("C4", "Encrypter"); exist {
		if strings.EqualFold(compress, "None") {
			c4_cfg.Compressor = event.ENCRYPTER_NONE
		}
	}
	c4_cfg.ConnectionPoolSize = 2
	if poosize, exist := common.Cfg.GetIntProperty("C4", "ConnectionPoolSize"); exist {
		c4_cfg.ConnectionPoolSize = uint32(poosize)
	}

	c4_cfg.MinWritePeriod = 500
	if period, exist := common.Cfg.GetIntProperty("C4", "MinWritePeriod"); exist {
		c4_cfg.MinWritePeriod = uint32(period)
	}
}

func (manager *C4) Init() error {
	log.Println("Init C4.")
	initC4Config()
	manager.idle_conns = make(chan RemoteConnection, c4_cfg.ConnectionPoolSize)
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
	}
	return nil
}

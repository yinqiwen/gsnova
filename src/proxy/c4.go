package proxy

import (
	"bytes"
	"common"
	//"container/list"
	"errors"
	"event"
	"io"
	"io/ioutil"
	"log"
	//"misc/upnp"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"util"
)

const (
	SESSION_INITED                   uint32 = 1
	SESSION_WAITING_CONNECT_RESPONSE uint32 = 2
	SESSION_WAITING_RESPONSE         uint32 = 3
	SESSION_TRANSACTION_COMPELETE    uint32 = 4
	SESSION_PROCEEDING               uint32 = 5
	SESSION_COMPLETED                uint32 = 6
	SESSION_DELETING                 uint32 = 7
)

var sessions map[uint32]*localProxySession = make(map[uint32]*localProxySession)

var bufMap map[string]*bytes.Buffer = make(map[string]*bytes.Buffer)
var readChanMap map[string]chan event.Event = make(map[string]chan event.Event)

type localProxySession struct {
	id          uint32
	server      string
	remote_addr string
	localConn   *SessionConnection
}

func (sess *localProxySession) requestEvent(ev event.Event) {
	evch := readChanMap[sess.server]
	//	switch ev.GetType() {
	//	case event.HTTP_REQUEST_EVENT_TYPE, event.EVENT_TCP_CHUNK_TYPE:
	//		var compress event.CompressEventV2
	//		compress.SetHash(ev.GetHash())
	//		compress.Ev = ev
	//		compress.CompressType = c4_cfg.Compressor
	//		ev = &compress
	//	}
	var encrypt event.EncryptEventV2
	encrypt.SetHash(ev.GetHash())
	encrypt.EncryptType = c4_cfg.Encrypter
	encrypt.Ev = ev
	//log.Printf("[%d]Block enter with chan len:%d\n", sess.id, len(evch))
	evch <- &encrypt
	//log.Printf("[%d]Block exit\n", sess.id)
}

func remote_loop(remote string) {
	baseDuration := time.Duration(c4_cfg.MinWritePeriod) * time.Millisecond
	tick := time.NewTicker(baseDuration)
	//    tickerMap[remote] = tick
	buf := new(bytes.Buffer)
	bufMap[remote] = buf
	read := make(chan event.Event, 4096)
	readChanMap[remote] = read
	for {
		select {
		case <-tick.C:
			req := &http.Request{
				Method:        "POST",
				URL:           &url.URL{Scheme: "http", Host: remote, Path: "/invoke"},
				Host:          remote,
				Header:        make(http.Header),
				Body:          ioutil.NopCloser(buf),
				ContentLength: int64(buf.Len()),
			}
			ifs, _ := net.Interfaces()
			req.Header.Set("UserToken", ifs[0].HardwareAddr.String())
			req.Header.Set("Connection", "keep-alive")
			req.Header.Set("Content-Type", "application/octet-stream")
			if len(c4_cfg.UA) > 0 {
				req.Header.Set("User-Agent", c4_cfg.UA)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Println(err.Error())
				continue
			}
			if resp.StatusCode != 200 {
				log.Printf("Unexpected response %d %s\n", resp.StatusCode, resp.Status)
				continue
			}
			buf.Reset()
			// write http response response to conn
			//io.Copy(conn, resp.Body)
			if resp.ContentLength > 0 {
				content := make([]byte, resp.ContentLength)
				n, err := io.ReadFull(resp.Body, content)
				if int64(n) != resp.ContentLength || nil != err {

				} else {
					go handleRecvBody(bytes.NewBuffer(content), remote)
				}
			}
			resp.Body.Close()
		case b := <-read:
			event.EncodeEvent(buf, b)
		}
	}
}

func handleRecvBody(buf *bytes.Buffer, server string) {
	//log.Printf("Handle content:%d\n", buf.Len())
	for buf.Len() > 0 {
		err, ev := event.DecodeEvent(buf)
		if nil == err {
			processRecvEvent(ev, server)
		}
	}
}

func getSession(id uint32) (*localProxySession, error) {
	handler, ok := sessions[id]
	if !ok {
		return nil, errors.New("Not exist session")
	}
	return handler, nil
}

func getCreateSession(ev *event.HTTPRequestEvent, server string) *localProxySession {
	handler, ok := sessions[ev.GetHash()]
	if !ok {
		handler = &(localProxySession{})
		handler.server = server
		sessions[ev.GetHash()] = handler
	}
	handler.remote_addr = ev.RawReq.Host
	if !strings.Contains(handler.remote_addr, ":") {
		handler.remote_addr = handler.remote_addr + ":80"
	}
	return handler
}

func deleteSessionEntry(id uint32) {
	handler, ok := sessions[id]
	if ok {
		handler.localConn.LocalRawConn.Close()
		delete(sessions, id)
	}

}

func processRecvEvent(ev event.Event, server string) error {
	ev = event.ExtractEvent(ev)
	//log.Printf("Start process event %T", ev)
	handler, err := getSession(ev.GetHash())
	if nil != err {
		if ev.GetHash() != 0 && ev.GetType() != event.EVENT_TCP_CONNECTION_TYPE {
			log.Printf("No session:%d found for %T\n", ev.GetHash(), ev)
			if nil != handler {

			}
		}
		return nil
	}
	//log.Printf("Session:%d process recv event:%T\n", ev.GetHash(), ev)
	switch ev.GetType() {
	case event.EVENT_TCP_CONNECTION_TYPE:
		cev := ev.(*event.SocketConnectionEvent)
		//		log.Printf("Status %d\n", cev.Status)
		if cev.Status == event.TCP_CONN_CLOSED {
			if cev.Addr == handler.remote_addr {
				deleteSessionEntry(ev.GetHash())
			}
		}
	case event.EVENT_TCP_CHUNK_TYPE:
		chunk := ev.(*event.TCPChunkEvent)
		_, err := handler.localConn.LocalRawConn.Write(chunk.Content)
		if nil != err {
			log.Printf("Failed to write  data to local client:%v.\n", err)
			deleteSessionEntry(ev.GetHash())
		}
	default:
		log.Printf("Unexpected event type:%d\n", ev.GetType())
	}
	return nil
}

type C4Config struct {
	Compressor         uint32
	Encrypter          uint32
	UA                 string
	ConnectionPoolSize uint32
	MinWritePeriod     uint32
}

var c4_cfg *C4Config

type C4HttpConnection struct {
	lastSessionId uint32
	manager       *C4
}

func (c4 *C4HttpConnection) Close() error {
	session, err := getSession(c4.lastSessionId)
	if nil == err {
		closeEv := &event.SocketConnectionEvent{}
		closeEv.Status = event.TCP_CONN_CLOSED
		closeEv.SetHash(c4.lastSessionId)
		session.requestEvent(closeEv)
		deleteSessionEntry(c4.lastSessionId)
	}
	return nil
}

func (c4 *C4HttpConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	if nil != ev {
		c4.lastSessionId = ev.GetHash()
		switch ev.GetType() {
		case event.HTTP_REQUEST_EVENT_TYPE:
			req := ev.(*event.HTTPRequestEvent)
			handler := getCreateSession(req, c4.manager.servers.Select().(string))
			handler.localConn = conn
			if strings.EqualFold(req.RawReq.Method, "CONNECT") {
				conn.State = STATE_RECV_HTTP_CHUNK
			} else {
				conn.State = STATE_RECV_HTTP
				proxyURL, _ := url.Parse(req.Url)
				req.Url = proxyURL.Path
			}
			log.Printf("Session[%d]Request %s %s\n", ev.GetHash(), req.Method, req.Url)
			handler.requestEvent(req)
			if conn.State == STATE_RECV_HTTP {
				if req.RawReq.ContentLength > 0 {
					rest := req.RawReq.ContentLength
					tmpbuf := make([]byte, 8192)
					for rest > 0 {
						n, err := req.RawReq.Body.Read(tmpbuf)
						if nil == err {
							rest = rest - int64(n)
							chunk := &event.TCPChunkEvent{Content: tmpbuf[0:n]}
							chunk.SetHash(req.GetHash())
							handler.requestEvent(chunk)
						} else {
							break
						}
					}
				}
				req.RawReq.Body.Close()
			}
		case event.HTTP_CHUNK_EVENT_TYPE:
			handler, err := getSession(ev.GetHash())
			if nil != err {
				return err, nil
			}
			chunk := ev.(*event.HTTPChunkEvent)
			tcp_chunk := &event.TCPChunkEvent{Content: chunk.Content}
			tcp_chunk.SetHash(ev.GetHash())
			handler.requestEvent(tcp_chunk)
			conn.State = STATE_RECV_HTTP_CHUNK
		}
	}

	return nil, nil
}
func (c4 *C4HttpConnection) GetConnectionManager() RemoteConnectionManager {
	return c4.manager
}

type C4RSocketConnection struct {
}

type C4 struct {
	servers *util.ListSelector
}

func (manager *C4) RecycleRemoteConnection(conn RemoteConnection) {
	//do nothing
}

func (manager *C4) GetRemoteConnection(ev event.Event) (RemoteConnection, error) {
	conn := &C4HttpConnection{}
	conn.manager = manager
	return conn, nil
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
	if enable, exist := common.Cfg.GetIntProperty("C4", "Enable"); exist {
		if enable == 0 {
			return nil
		}
	}
	log.Println("Init C4.")
	initC4Config()
	RegisteRemoteConnManager(manager)
	//manager.auths = new(util.ListSelector)
	manager.servers = &util.ListSelector{}
//    nat, err := upnp.Discover()
//    if nil == err{
//       err = nat.AddPortMapping("tcp", 48101, 48101, "GSnova", 3000)
//       err = nat.AddPortMapping("udp", 48101, 48101, "GSnova", 3000)
//    }
//    if nil != err {
//       log.Printf("Failed to discover:%v\n", err)
//    }
	index := 0
	for {
		v, exist := common.Cfg.GetProperty("C4", "WorkerNode["+strconv.Itoa(index)+"]")
		if !exist || len(v) == 0 {
			break
		}
		manager.servers.Add(v)
		go remote_loop(v)
		index = index + 1
	}
	//no appid found, fetch shared from master
	if index == 0 {
		return errors.New("No configed C4 server.")
	}
	return nil
}

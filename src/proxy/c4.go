package proxy

import (
	"bytes"
	"common"
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

var c4_enable bool
var logined bool
var userToken string
var externIP string
var sessions map[uint32]*localProxySession = make(map[uint32]*localProxySession)

var readChanMap map[string][]chan event.Event = make(map[string][]chan event.Event)

func getRequestChan(server string, ev event.Event) chan event.Event {
	index := int(ev.GetHash()) % len(readChanMap[server])
	return readChanMap[server][index]
}

type localProxySession struct {
	closed        bool
	id            uint32
	server        string
	remote_addr   string
	localConn     *SessionConnection
	writeBackChan chan event.Event
}

func (sess *localProxySession) loop() {
	tick := time.NewTicker(10 * time.Millisecond)
	for !sess.closed {
		select {
		case <-tick.C:
			break
		case ev := <-sess.writeBackChan:
			if nil == ev {
				return
			}
			processRecvEvent(ev, sess.server)
		}
	}
}

func (sess *localProxySession) requestEvent(ev event.Event) {
	switch ev.GetType() {
	case event.EVENT_TCP_CHUNK_TYPE:
		var compress event.CompressEventV2
		compress.SetHash(ev.GetHash())
		compress.Ev = ev
		compress.CompressType = c4_cfg.Compressor
		ev = &compress
	}
	var encrypt event.EncryptEventV2
	encrypt.SetHash(ev.GetHash())
	encrypt.EncryptType = c4_cfg.Encrypter
	encrypt.Ev = ev
	//log.Printf("[%d]Block enter with chan len:%d\n", sess.id, len(evch))
	getRequestChan(sess.server, ev) <- &encrypt
	//log.Printf("[%d]Block exit\n", sess.id)
}

func http_remote_loop(remote string, index int) {
	baseDuration := time.Duration(c4_cfg.MinWritePeriod) * time.Millisecond
	tick := time.NewTicker(baseDuration)
	//    tickerMap[remote] = tick
	buf := new(bytes.Buffer)
	//bufMap[remote] = buf
	read := make(chan event.Event, 4096)
	readChanMap[remote][index] = read
	if !logined {
		login := &event.UserLoginEvent{}
		login.User = userToken
		read <- login
		logined = true
	}
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
			req.Header.Set("UserToken", userToken)
			//req.Header.Set("Connection", "close")
			req.Header.Set("Connection", "keep-alive")
			req.Header.Set("FetcherIndex", fmt.Sprintf("%d:%d", index, c4_cfg.ConnectionPoolSize))
			//req.Header.Set("Keep-Alive", "timeout=60, max=100")
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
				log.Printf("Unexpected response %s for %s\n", resp.Status, remote)
				continue
			}
			
			// write http response response to conn
			//io.Copy(conn, resp.Body)
			if resp.ContentLength > 0 {
				content := make([]byte, resp.ContentLength)
				n, err := io.ReadFull(resp.Body, content)
				if int64(n) != resp.ContentLength || nil != err {
					log.Printf("Failed to read data from body %d or %v", n, err)
					continue
				} else {
					go handleRecvBody(bytes.NewBuffer(content), remote, index)
				}
			}
			buf.Reset()
			resp.Body.Close()
		case b := <-read:
			event.EncodeEvent(buf, b)
		}
	}
}

func handleRecvBody(buf *bytes.Buffer, server string, index int) {
	//log.Printf("Handle content:%d\n", buf.Len())
	for buf.Len() > 0 {
		err, ev := event.DecodeEvent(buf)
		if nil == err {
			//processRecvEvent(ev, server)
			sess, err := getSession(ev.GetHash())
			if nil == err {
				sess.writeBackChan <- ev
			} else {
				ev = event.ExtractEvent(ev)
				if ev.GetHash() != 0 && ev.GetType() != event.EVENT_TCP_CONNECTION_TYPE {
					log.Printf("No session:%d found for %T\n", ev.GetHash(), ev)
					closeEv := &event.SocketConnectionEvent{}
					closeEv.Status = event.TCP_CONN_CLOSED
					closeEv.SetHash(ev.GetHash())
					readChanMap[server][index] <- closeEv
				}
			}
		}
	}
	buf.Reset()
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
		handler.closed = false
		handler.writeBackChan = make(chan event.Event, 4096)
		sessions[ev.GetHash()] = handler
		go handler.loop()
	}

	return handler
}

func deleteSessionEntry(id uint32) {
	handler, ok := sessions[id]
	if ok {
		handler.closed = true
		//close(handler.writeBackChan)
		handler.localConn.LocalRawConn.Close()
		delete(sessions, id)
	}
}

func processRecvEvent(ev event.Event, server string) error {
	ev = event.ExtractEvent(ev)
	//log.Printf("Start process event %T", ev)
	handler, err := getSession(ev.GetHash())
	if nil != err {

		return nil
	}
	//log.Printf("Recv event type:%T\n", ev)
	switch ev.GetType() {
	case event.EVENT_TCP_CONNECTION_TYPE:
		cev := ev.(*event.SocketConnectionEvent)
		//		log.Printf("Status %d\n", cev.Status)
		if cev.Status == event.TCP_CONN_CLOSED {
			if cev.Addr == handler.remote_addr {
				log.Printf("[%d]Close session for %s\n", ev.GetHash(), cev.Addr)
				deleteSessionEntry(ev.GetHash())
			}
		}
	case event.EVENT_TCP_CHUNK_TYPE:
		chunk := ev.(*event.TCPChunkEvent)
		//log.Printf("[%d]Write Chunk:%d with %d bytes", ev.GetHash(), chunk.Sequence, len(chunk.Content))
		n, err := handler.localConn.LocalRawConn.Write(chunk.Content)
		if nil != err {
			log.Printf("[%d]Failed to write  data to local client:%v.\n", ev.GetHash(), err)
			deleteSessionEntry(ev.GetHash())
			closeEv := &event.SocketConnectionEvent{}
			closeEv.Status = event.TCP_CONN_CLOSED
			closeEv.SetHash(ev.GetHash())
			getRequestChan(server, ev) <- closeEv
		}
		if nil == err && n != len(chunk.Content) {
			log.Printf("[%d]=================less data=======.\n", ev.GetHash())
		}
	default:
		log.Printf("Unexpected event type:%d\n", ev.GetType())
	}
	return nil
}

type C4Config struct {
	Compressor             uint32
	Encrypter              uint32
	UA                     string
	ConnectionMode         string
	ConnectionPoolSize     uint32
	MinWritePeriod         uint32
	RSocketPort            uint32
	RSocketHeartBeatPeriod uint32
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
			handler.remote_addr = req.RawReq.Host
			if !strings.Contains(handler.remote_addr, ":") {
				handler.remote_addr = handler.remote_addr + ":80"
			}
			handler.localConn = conn
			if strings.EqualFold(req.RawReq.Method, "CONNECT") {
				conn.State = STATE_RECV_HTTP_CHUNK
			} else {
				conn.State = STATE_RECV_HTTP
				proxyURL, _ := url.Parse(req.Url)
				req.Url = proxyURL.RequestURI()
			}
			log.Printf("Session[%d]Request %s\n", req.GetHash(), util.GetURLString(req.RawReq, true))
			//log.Printf("Session[%d]Request %s %s\n", ev.GetHash(), req.Method, req.Url)
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
	if period, exist := common.Cfg.GetIntProperty("C4", "HTTPMinWritePeriod"); exist {
		c4_cfg.MinWritePeriod = uint32(period)
	}

	c4_cfg.ConnectionMode = MODE_HTTP

	logined = false
	ifs, _ := net.Interfaces()
	userToken = ifs[0].HardwareAddr.String()
	log.Printf("UserToken is %s\n", userToken)
}

func (manager *C4) Init() error {
	if enable, exist := common.Cfg.GetIntProperty("C4", "Enable"); exist {
		c4_enable = (enable != 0)
		if enable == 0 {
			return nil
		}
	}

	log.Println("Init C4.")
	initC4Config()
	RegisteRemoteConnManager(manager)
	manager.servers = &util.ListSelector{}

	index := 0
	for {
		v, exist := common.Cfg.GetProperty("C4", "WorkerNode["+strconv.Itoa(index)+"]")
		if !exist || len(v) == 0 {
			break
		}
		manager.servers.Add(v)
		readChanMap[v] = make([]chan event.Event, c4_cfg.ConnectionPoolSize)
		for i := 0; i < int(c4_cfg.ConnectionPoolSize); i++ {
			readChanMap[v][i] = make(chan event.Event, 1024)
		}
		if strings.EqualFold(c4_cfg.ConnectionMode, MODE_HTTP) {
			for i := 0; i < int(c4_cfg.ConnectionPoolSize); i++ {
				go http_remote_loop(v, i)
			}
		}
		index = index + 1
	}
	if index == 0 {
		return errors.New("No configed C4 server.")
	}
	return nil
}

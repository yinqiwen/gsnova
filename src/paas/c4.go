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
	"sync/atomic"
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

var handlers []map[uint32]*C4SessionHandler
var recvEvChan chan event.Event

func getSessionHandler(ev event.Event) (*C4SessionHandler, map[uint32]*C4SessionHandler) {
	index := ev.GetHash() % c4_cfg.ConnectionPoolSize
	handler, ok := handlers[index][ev.GetHash()]
	if !ok {
		return nil, handlers[index]
	}
	return handler, handlers[index]
}

func firstSessionHandler(ev event.Event) *C4SessionHandler {
	index := ev.GetHash() % c4_cfg.ConnectionPoolSize
	for _, v := range handlers[index] {
		return v
	}
	return nil
}

func getCreateSessionHandler(ev event.Event, conn *C4HttpConnection) *C4SessionHandler {
	index := ev.GetHash() % c4_cfg.ConnectionPoolSize
	handler, ok := handlers[index][ev.GetHash()]
	if !ok {
		handler = &(C4SessionHandler{})
		handler.server = conn.server
		handler.clear()
		handlers[index][ev.GetHash()] = handler
	}
	return handler
}

func recvEventLoop() {
	for {
		ev := <-recvEvChan
		processRecvEvent(ev)
	}
}

func delayDeleteSessionEntry(m map[uint32]*C4SessionHandler, h *C4SessionHandler) {
	time.Sleep(60 * time.Second)
	h.conn.LocalRawConn.Close()
	delete(m, h.conn.SessionID)
}

func processRecvEvent(ev event.Event) error {
	handler, handler_map := getSessionHandler(ev)
	if nil == handler {
		if ev.GetHash() != 0 && ev.GetType() != event.HTTP_CONNECTION_EVENT_TYPE {
			log.Printf("No session:%d found for %T\n", ev.GetHash(), ev)
			handler = firstSessionHandler(ev)
			if nil != handler {
				conn := handler.conn.RemoteConn.(*C4HttpConnection)
				closeEv := &event.HTTPConnectionEvent{}
				closeEv.Status = event.HTTP_CONN_CLOSED
				closeEv.SetHash(ev.GetHash())
				conn.requestEvent(closeEv)
			}
		}
		return nil
	}
	log.Printf("Session:%d process recv event:%T\n", ev.GetHash(), ev)
	switch ev.GetType() {
	case event.HTTP_RESPONSE_EVENT_TYPE:
		res := ev.(*event.HTTPResponseEvent)
		res.ToResponse().Write(handler.conn.LocalRawConn)
	case event.EVENT_SEQUNCEIAL_CHUNK_TYPE:
		chunk := ev.(*event.SequentialChunkEvent)
		handler.seqMap[chunk.Sequence] = chunk
		for {
			chunk, ok := handler.seqMap[handler.lastWriteLocalSeq]
			if !ok {
				break
			}
			handler.conn.LocalRawConn.Write(chunk.Content)
			delete(handler.seqMap, handler.lastWriteLocalSeq)
			handler.lastWriteLocalSeq = atomic.AddUint32(&handler.lastWriteLocalSeq, 1)
		}
	case event.EVENT_TRANSACTION_COMPLETE_TYPE:
		handler.state = SESSION_TRANSACTION_COMPELETE
	case event.EVENT_REST_NOTIFY_TYPE:
		//		rest := ev.(*event.EventRestNotify)
		//		if rest.Rest > 0 {
		//			log.Printf("Rest %d\n", rest.Rest)
		//		}
	case event.HTTP_CONNECTION_EVENT_TYPE:
		cev := ev.(*event.HTTPConnectionEvent)
		//		log.Printf("Status %d\n", cev.Status)
		if cev.Status == event.HTTP_CONN_CLOSED {
			handler.state = SESSION_DELETING
			go delayDeleteSessionEntry(handler_map, handler)
			//delete(handler_map, ev.GetHash())
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

type C4HttpAssistConnection struct {
	server  string
	manager *C4
	ev_list *list.List
	client  *http.Client
}

type C4SessionHandler struct {
	conn              *SessionConnection
	lastWriteLocalSeq uint32
	lastReadLocalSeq  uint32
	seqMap            map[uint32]*event.SequentialChunkEvent
	isHttps           bool
	state             uint32
	server            string
}

func (handler *C4SessionHandler) clear() {
	handler.lastWriteLocalSeq = 0
	handler.lastReadLocalSeq = 0
	handler.isHttps = false
	handler.seqMap = make(map[uint32]*event.SequentialChunkEvent)
	handler.state = SESSION_INITED
}

type C4HttpConnection struct {
	server      string
	manager     *C4
	client      []*http.Client
	assit       *http.Client
	sendEvChans []chan event.Event
}

func (c4 *C4HttpConnection) Close() error {
	return nil
}

func (c4 *C4HttpConnection) Auth() error {
	c4.assit = &(http.Client{})
	c4.client = make([]*http.Client, c4_cfg.ConnectionPoolSize)
	c4.sendEvChans = make([]chan event.Event, c4_cfg.ConnectionPoolSize)

	for i := 0; i < int(c4_cfg.ConnectionPoolSize); i++ {
		c4.client[i] = &(http.Client{})
		c4.sendEvChans[i] = make(chan event.Event, 1024)
		go c4.process(i)
	}

	go c4.assistLoop()
	return nil
}

func (c4 *C4HttpConnection) assistLoop() {
	for {
		var tmp bytes.Buffer
		req := &(event.EventRestRequest{})
		req.RestSessions = make([]uint32, 0)
		for _, hmap := range handlers {
			for _, v := range hmap {
				if v.server == c4.server && v.state != SESSION_TRANSACTION_COMPELETE && v.state != SESSION_DELETING {
					req.RestSessions = append(req.RestSessions, uint32(v.conn.SessionID))
				}
			}
		}
		event.EncodeEvent(&tmp, req)
		c4.processClient(c4.assit, &tmp, true)
		//time.Sleep(time.Duration(c4_cfg.MinWritePeriod) * time.Millisecond)
	}
}

func (c4 *C4HttpConnection) processClient(cli *http.Client, buf *bytes.Buffer, isAssist bool) error {
	//log.Println("#######processClient")
	//	if buf.Len() == 0 {
	//		tmp := &(event.EventRestRequest{})
	//		tmp.SetHash(0)
	//		event.EncodeEvent(buf, tmp)
	//	}
	req := &http.Request{
		Method:        "POST",
		URL:           &url.URL{Scheme: "http", Host: c4.server, Path: "/invoke"},
		Host:          c4.server,
		Header:        make(http.Header),
		Body:          ioutil.NopCloser(buf),
		ContentLength: int64(buf.Len()),
	}
	if len(c4_cfg.UA) > 0 {
		req.Header.Set("User-Agent", c4_cfg.UA)
	}
	req.Close = false
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/octet-stream")
	if isAssist {
		req.Header.Set("ClientActor", "Assist")
	} else {
		req.Header.Set("ClientActor", "Primary")
	}
	//req.Header.Set("UserToken", "123456")
	req.Header.Set("MaxResponseSize", strconv.Itoa(512*1024))
	if response, err := cli.Do(req); nil != err {
		log.Printf("Failed to request data from C4:%s for reason:%s\n", c4.server, err.Error())
		return err
	} else {
		if response.StatusCode != 200 {
			log.Printf("Invalid response:%d from server:%s\n", response.StatusCode, c4.server)
			return errors.New("Invalid response")
		} else {
			//log.Printf("Response with content len:%d while request body:%d\n", response.ContentLength, req.ContentLength)
			content := make([]byte, response.ContentLength)
			n, err := io.ReadFull(response.Body, content)
			if int64(n) < response.ContentLength {
				return errors.New("No sufficient space in body.")
			}
			if nil != err {
				log.Printf("Failed to read body:%s\n", err.Error())
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
					//log.Printf("Recv event:%T\n", res)
					recvEvChan <- res
				} else {
					log.Printf("Invalid event:%s\n", err.Error())
					return err
				}
			}
		}
	}
	return nil
}

func (c4 *C4HttpConnection) process(index int) error {
	client := c4.client[index]
	sendEvChan := c4.sendEvChans[index]
	for {
		var buf bytes.Buffer
		select {
		case ev := <-sendEvChan:
			event.EncodeEvent(&buf, ev)
		default:
			break
		}
		if buf.Len() == 0 {
			time.Sleep(time.Duration(c4_cfg.MinWritePeriod) * time.Millisecond)
		}
		c4.processClient(client, &buf, false)
	}
	return nil
}

func (c4 *C4HttpConnection) requestEvent(ev event.Event) {
	index := ev.GetHash() % c4_cfg.ConnectionPoolSize
	var compress event.CompressEventV2
	compress.SetHash(ev.GetHash())
	compress.Ev = ev
	compress.CompressType = c4_cfg.Compressor
	var encrypt event.EncryptEventV2
	encrypt.SetHash(ev.GetHash())
	encrypt.EncryptType = c4_cfg.Encrypter
	encrypt.Ev = &compress
	c4.sendEvChans[index] <- &encrypt
}

func (c4 *C4HttpConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	if nil != ev {
		handler := getCreateSessionHandler(ev, c4)
		handler.conn = conn
		switch ev.GetType() {
		case event.HTTP_REQUEST_EVENT_TYPE:
			req := ev.(*event.HTTPRequestEvent)
			if strings.EqualFold(req.RawReq.Method, "CONNECT") {
				handler.isHttps = true
				conn.State = STATE_RECV_HTTP_CHUNK
			} else {
				conn.State = STATE_RECV_HTTP
				handler.clear()
				proxyURL, _ := url.Parse(req.Url)
				req.Url = proxyURL.Path
			}
			log.Printf("Session[%d]Request %s %s\n", ev.GetHash(), req.Method, req.Url)
			c4.requestEvent(req)
			if conn.State == STATE_RECV_HTTP {
				if req.RawReq.ContentLength > 0 {
					tmpbuf := make([]byte, 8192)
					for {
						n, err := req.RawReq.Body.Read(tmpbuf)
						if nil == err {
							seq := &(event.SequentialChunkEvent{Content: tmpbuf[0:n]})
							seq.SetHash(req.GetHash())
							seq.Sequence = handler.lastReadLocalSeq
							handler.lastReadLocalSeq = atomic.AddUint32(&handler.lastReadLocalSeq, 1)
							c4.requestEvent(seq)
						} else {
							break
						}
					}
				}
			}
		case event.HTTP_CHUNK_EVENT_TYPE:
			chunk := ev.(*event.HTTPChunkEvent)
			seq := &(event.SequentialChunkEvent{Content: chunk.Content})
			seq.SetHash(chunk.GetHash())
			seq.Sequence = handler.lastReadLocalSeq
			handler.lastReadLocalSeq = atomic.AddUint32(&handler.lastReadLocalSeq, 1)
			c4.requestEvent(seq)
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
	//auths      *util.ListSelector
	conns []*util.ListSelector

	//idle_conns chan RemoteConnection
}

//func (manager *C4) recycleRemoteConnection(conn RemoteConnection) {
//	select {
//	case manager.idle_conns <- conn:
//		// Buffer on free list; nothing more to do.
//	default:
//		// Free list full, just carry on.
//	}
//}

func (manager *C4) RecycleRemoteConnection(conn RemoteConnection) {
	//do nothing
}

func (manager *C4) GetRemoteConnection(ev event.Event) (RemoteConnection, error) {
	index := int(ev.GetHash()) % len(manager.conns)
	return manager.conns[index].Select().(RemoteConnection), nil
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
	manager.conns = make([]*util.ListSelector, 0)
	handlers = make([]map[uint32]*C4SessionHandler, c4_cfg.ConnectionPoolSize)
	for i := 0; i < int(c4_cfg.ConnectionPoolSize); i++ {
		handlers[i] = make(map[uint32]*C4SessionHandler)
	}
	recvEvChan = make(chan event.Event)

	index := 0
	for {
		v, exist := common.Cfg.GetProperty("C4", "WorkerNode["+strconv.Itoa(index)+"]")
		if !exist || len(v) == 0 {
			break
		}
		manager.conns = append(manager.conns, &util.ListSelector{})
		for i := 0; i < int(c4_cfg.ConnectionPoolSize); i++ {
			conn := new(C4HttpConnection)
			conn.server = strings.TrimSpace(v)
			conn.manager = manager
			if err := conn.Auth(); nil != err {
				log.Printf("Failed to auth server:%s\n", err.Error())
				return err
			}
			manager.conns[index].Add(conn)
		}
		index = index + 1
	}
	//no appid found, fetch shared from master
	if index == 0 {
		return errors.New("No configed C4 server.")
	}
	go recvEventLoop()
	return nil
}

package proxy

import (
	"bytes"
	"common"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"event"
	//"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"util"
)

var C4Enable bool
var logined bool
var userToken string

var total_c4_conn_num = int32(0)
var total_c4_routines = int32(0)

type C4Config struct {
	Compressor             uint32
	Encrypter              uint32
	UA                     string
	ConnectionMode         string
	ReadTimeout            uint32
	MaxConn                uint32
	WSConnKeepAlive        uint32
	InjectRange            []*regexp.Regexp
	FetchLimitSize         uint32
	ConcurrentRangeFetcher uint32
	Proxy                  string
	MultiRangeFetchEnable  bool
	UseSysDNS              bool
}

var c4_cfg *C4Config
var c4HttpClient *http.Client

var c4SessionTable = make(map[uint32]*C4RemoteSession)
var c4SessionTableMutex sync.Mutex
var c4WriteCBChannels = make(map[uint32]chan event.Event)

type C4CumulateTask struct {
	chunkLen int32
	buffer   bytes.Buffer
}

func (task *C4CumulateTask) fillContent(reader io.Reader) error {
	data := make([]byte, 8192)
	for {
		n, err := reader.Read(data)
		if nil != err {
			return err
		}
		task.buffer.Write(data[0:n])
		for {
			if task.chunkLen < 0 && task.buffer.Len() >= 4 {
				err = binary.Read(&task.buffer, binary.BigEndian, &task.chunkLen)
				if nil != err {
					log.Printf("#################%v\n", err)
					break
				}
			}
			if task.chunkLen >= 0 && task.buffer.Len() >= int(task.chunkLen) {
				content := task.buffer.Next(int(task.chunkLen))
				tmp := bytes.NewBuffer(content)
				err, evv := event.DecodeEvent(tmp)
				if nil == err {
					evv = event.ExtractEvent(evv)
					idx := evv.GetHash() % uint32(len(c4WriteCBChannels))
					c4WriteCBChannels[idx] <- evv
					//					c4 := getC4Session(evv.GetHash())
					//					if nil == c4 {
					//						if evv.GetType() != event.EVENT_TCP_CONNECTION_TYPE {
					//							log.Printf("[ERROR]No C4 session found for %d with type:%T\n", evv.GetHash(), evv)
					//						}
					//					} else {
					//						c4.handleTunnelResponse(c4.sess, evv)
					//					}
				} else {
					log.Printf("[ERROR]Decode event failed %v with content len:%d\n", err, task.chunkLen)
				}
				task.buffer.Truncate(task.buffer.Len())
				task.chunkLen = -1
			} else {
				break
			}
		}
	}
	return nil
}

type C4RemoteSession struct {
	sess                   *SessionConnection
	remote_proxy_addr      string
	server                 string
	closed                 bool
	manager                *C4
	injectRange            bool
	remoteHttpClientEnable bool
	rangeWorker            *rangeFetchTask
}

func (c4 *C4RemoteSession) Close() error {
	if !c4.closed {
		closeEv := &event.SocketConnectionEvent{}
		closeEv.Status = event.TCP_CONN_CLOSED
		closeEv.SetHash(c4.sess.SessionID)
		c4.offerRequestEvent(closeEv)
	}
	c4.closed = true
	removeC4SessionTable(c4)
	if nil != c4.rangeWorker {
		c4.rangeWorker.Close()
	}
	return nil
}

func wrapC4RequestEvent(ev event.Event) event.Event {
	var encrypt event.EncryptEventV2
	encrypt.SetHash(ev.GetHash())
	encrypt.EncryptType = c4_cfg.Encrypter
	encrypt.Ev = ev
	return &encrypt
}

func (c4 *C4RemoteSession) offerRequestEvent(ev event.Event) {
	ev = wrapC4RequestEvent(ev)
	isWsServer := strings.HasPrefix(c4.server, "ws://")
	if isWsServer {
		wsOfferEvent(c4.server, ev)
		return
	}
	httpOfferEvent(c4.server, ev)
}

func writeCBLoop(index uint32) {
	ch := c4WriteCBChannels[index]
	for {
		select {
		case ev := <-ch:
			if nil != ev {
				c4 := getC4Session(ev.GetHash())
				if nil != c4 {
					c4.handleTunnelResponse(c4.sess, ev)
				} else {
					if ev.GetType() != event.EVENT_TCP_CONNECTION_TYPE {
						log.Printf("[ERROR]No session[%d] found for %T\n", ev.GetHash(), ev)
					}
				}
			}
		}
	}
}

func (c4 *C4RemoteSession) handleTunnelResponse(conn *SessionConnection, ev event.Event) error {
	switch ev.GetType() {
	case event.EVENT_TCP_CONNECTION_TYPE:
		cev := ev.(*event.SocketConnectionEvent)
		if cev.Status == event.TCP_CONN_CLOSED {
			log.Printf("Session[%d]Remote %s connection closed, current proxy addr:%s\n", conn.SessionID, cev.Addr, c4.remote_proxy_addr)
			if c4.remote_proxy_addr == cev.Addr {
				c4.closed = true
				conn.Close()
				c4.Close()
				return io.EOF
			}
		}
	case event.EVENT_TCP_CHUNK_TYPE:
		chunk := ev.(*event.TCPChunkEvent)
		log.Printf("Session[%d]Handle TCP chunk[%d-%d]\n", conn.SessionID, chunk.Sequence, len(chunk.Content))
		n, err := conn.LocalRawConn.Write(chunk.Content)
		if nil != err {
			log.Printf("[%d]Failed to write  chunk[%d-%d] to local client:%v.\n", ev.GetHash(), chunk.Sequence, len(chunk.Content), err)
			conn.Close()
			c4.Close()
			return err
		}
		if n < len(chunk.Content) {
			log.Printf("[%d]Less data written[%d-%d] to local client.\n", ev.GetHash(), n, len(chunk.Content))
		}
		chunk.Content = nil
	case event.HTTP_RESPONSE_EVENT_TYPE:
		log.Printf("Session[%d]Handle HTTP Response event with range task:%p\n", conn.SessionID, c4.rangeWorker)
		res := ev.(*event.HTTPResponseEvent)
		httpres := res.ToResponse()
		//log.Printf("Session[%d]Recv res %d %v\n", ev.GetHash(), httpres.StatusCode, httpres.Header)
		if nil != c4.rangeWorker {
			httpWriter := func(preq *http.Request) error {
				return c4.writeHttpRequest(preq)
			}
			pres, err := c4.rangeWorker.ProcessAyncResponse(httpres, httpWriter)
			if nil == err {
				if nil != pres {
					//log.Printf("Session[%d] %d %v\n", ev.GetHash(), pres.StatusCode, pres.Header)
					go pres.Write(conn.LocalRawConn)
				} else {
					//log.Printf("Session[%d]NULLL\n", ev.GetHash())
				}
			} else {
				log.Printf("####%v\n", err)
				c4.Close()
				conn.Close()
			}
		} else {
			httpres.Write(conn.LocalRawConn)
		}
	default:
		log.Printf("Unexpected event type:%d\n", ev.GetType())
	}
	return nil
}

func (c4 *C4RemoteSession) writeHttpRequest(preq *http.Request) error {
	ev := new(event.HTTPRequestEvent)
	ev.FromRequest(preq)
	ev.SetHash(c4.sess.SessionID)
	if strings.Contains(ev.Url, "http://") {
		ev.Url = ev.Url[7+len(preq.Host):]
	}
	//log.Printf("Session[%d]Range Request %s\n", c4.sess.SessionID, ev.Url)
	c4.offerRequestEvent(ev)
	return nil
}

func (c4 *C4RemoteSession) doRangeFetch(req *http.Request) error {
	task := new(rangeFetchTask)
	task.FetchLimit = int(c4_cfg.FetchLimitSize)
	task.FetchWorkerNum = int(c4_cfg.ConcurrentRangeFetcher)
	task.SessionID = c4.sess.SessionID
	c4.rangeWorker = task
	//	rh := req.Header.Get("Range")
	//	if len(rh) > 0 {
	//		log.Printf("Session[%d] Request  has range:%s\n", c4.sess.SessionID, rh)
	//	} else {
	//		log.Printf("Session[%d] Request has no range\n", c4.sess.SessionID)
	//	}
	httpWriter := func(preq *http.Request) error {
		return c4.writeHttpRequest(preq)
	}
	return task.AyncGet(req, httpWriter)
}

func (c4 *C4RemoteSession) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	c4.sess = conn
	if len(c4.server) == 0 {
		c4.server = c4.manager.servers.Select().(string)
	}
	c4.sess = conn
	c4.closed = false
	setC4SessionTable(c4)
	switch ev.GetType() {
	case event.HTTP_REQUEST_EVENT_TYPE:
		req := ev.(*event.HTTPRequestEvent)
		default_port := "80"
		if strings.EqualFold(req.RawReq.Method, "CONNECT") {
			conn.State = STATE_RECV_HTTP_CHUNK
			default_port = "443"
		} else {
			conn.State = STATE_RECV_HTTP
		}
		log.Printf("Session[%d] Request %s\n", req.GetHash(), util.GetURLString(req.RawReq, true))
		if nil != err {
			log.Printf("Session[%d]Failed to encode request to bytes", req.GetHash())
			return
		}
		remote_addr := req.RawReq.Host
		if !strings.Contains(remote_addr, ":") {
			remote_addr = net.JoinHostPort(req.RawReq.Host, default_port)
		}
		if strings.Contains(req.Url, "http://") {
			req.Url = req.Url[7+len(req.RawReq.Host):]
		}
		c4.rangeWorker = nil
		c4.remote_proxy_addr = remote_addr
		if strings.EqualFold(req.Method, "GET") && c4_cfg.MultiRangeFetchEnable {
			if c4.injectRange || hostPatternMatched(c4_cfg.InjectRange, req.RawReq.Host) {
				if nil == c4.doRangeFetch(req.RawReq) {
					return nil, nil
				}
			}
		}

		c4.offerRequestEvent(req)
		rest := req.RawReq.ContentLength
		tmpbuf := make([]byte, 8192)
		for rest > 0 {
			n, err := req.RawReq.Body.Read(tmpbuf)
			if nil == err {
				rest = rest - int64(n)
				chunk := &event.TCPChunkEvent{Content: tmpbuf[0:n]}
				chunk.SetHash(req.GetHash())
				c4.offerRequestEvent(chunk)
			} else {
				break
			}
		}
		req.RawReq.Body.Close()
	case event.HTTP_CHUNK_EVENT_TYPE:
		//log.Printf("Session[%d]Offer chunk\n", conn.SessionID)
		chunk := ev.(*event.HTTPChunkEvent)
		tcp_chunk := &event.TCPChunkEvent{Content: chunk.Content}
		tcp_chunk.SetHash(ev.GetHash())
		c4.offerRequestEvent(tcp_chunk)
		conn.State = STATE_RECV_HTTP_CHUNK
	}
	return nil, nil
}
func (c4 *C4RemoteSession) GetConnectionManager() RemoteConnectionManager {
	return c4.manager
}

type C4 struct {
	servers *util.ListSelector
}

func setC4SessionTable(c4 *C4RemoteSession) {
	c4SessionTableMutex.Lock()
	c4SessionTable[c4.sess.SessionID] = c4
	c4SessionTableMutex.Unlock()
}

func removeC4SessionTable(c4 *C4RemoteSession) {
	c4SessionTableMutex.Lock()
	delete(c4SessionTable, c4.sess.SessionID)
	c4SessionTableMutex.Unlock()
}

func getC4Session(sid uint32) *C4RemoteSession {
	c4SessionTableMutex.Lock()
	defer c4SessionTableMutex.Unlock()
	s, ok := c4SessionTable[sid]
	if ok {
		return s
	}
	return nil
}

func (manager *C4) RecycleRemoteConnection(conn RemoteConnection) {
	atomic.AddInt32(&total_c4_conn_num, -1)
}

func (manager *C4) loginC4(server string) {
	conn := &C4RemoteSession{}
	conn.manager = manager
	conn.server = server
	login := &event.UserLoginEvent{}
	login.User = userToken
	conn.offerRequestEvent(login)
}

func (manager *C4) GetRemoteConnection(ev event.Event, attrs map[string]string) (RemoteConnection, error) {
	conn := &C4RemoteSession{}
	conn.manager = manager

	found := false
	if containsAttr(attrs, ATTR_RANGE) {
		conn.injectRange = true
	} else {
		conn.injectRange = false
	}
	if containsAttr(attrs, ATTR_APP) {
		app := attrs[ATTR_APP]
		for _, tmp := range manager.servers.ArrayValues() {
			server := tmp.(string)
			if strings.Contains(server, app) {
				conn.server = server
				found = true
				break
			}
		}
	}
	if !found {
		conn.server = manager.servers.Select().(string)
	}

	atomic.AddInt32(&total_c4_conn_num, 1)
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
	if enc, exist := common.Cfg.GetProperty("C4", "Encrypter"); exist {
		enc = strings.ToLower(enc)
		switch enc {
		case "none":
			c4_cfg.Encrypter = event.ENCRYPTER_NONE
			//Support RC4 next release
		case "rc4":
			c4_cfg.Encrypter = event.ENCRYPTER_RC4
		}
	}

	c4_cfg.ReadTimeout = 25
	if period, exist := common.Cfg.GetIntProperty("C4", "ReadTimeout"); exist {
		c4_cfg.ReadTimeout = uint32(period)
	}

	c4_cfg.MaxConn = 5
	if num, exist := common.Cfg.GetIntProperty("C4", "MaxConn"); exist {
		c4_cfg.MaxConn = uint32(num)
	}

	c4_cfg.WSConnKeepAlive = 180
	if num, exist := common.Cfg.GetIntProperty("C4", "WSConnKeepAlive"); exist {
		c4_cfg.WSConnKeepAlive = uint32(num)
	}
	if tmp, exist := common.Cfg.GetProperty("C4", "Proxy"); exist {
		c4_cfg.Proxy = tmp
	}
	c4_cfg.ConcurrentRangeFetcher = 5
	if fetcher, exist := common.Cfg.GetIntProperty("C4", "RangeConcurrentFetcher"); exist {
		c4_cfg.ConcurrentRangeFetcher = uint32(fetcher)
	}
	c4_cfg.FetchLimitSize = 256000
	if limit, exist := common.Cfg.GetIntProperty("C4", "RangeFetchLimitSize"); exist {
		c4_cfg.FetchLimitSize = uint32(limit)
	}

	c4_cfg.MultiRangeFetchEnable = false
	if enable, exist := common.Cfg.GetIntProperty("C4", "MultiRangeFetchEnable"); exist {
		c4_cfg.MultiRangeFetchEnable = (enable != 0)
	}

	c4_cfg.UseSysDNS = false
	if enable, exist := common.Cfg.GetIntProperty("C4", "UseSysDNS"); exist {
		c4_cfg.UseSysDNS = (enable != 0)
	}

	c4_cfg.InjectRange = []*regexp.Regexp{}
	if ranges, exist := common.Cfg.GetProperty("C4", "InjectRange"); exist {
		c4_cfg.InjectRange = initHostMatchRegex(ranges)
	}
	logined = false
	if ifs, err := net.Interfaces(); nil == err {
		for _, itf := range ifs {
			if len(itf.HardwareAddr.String()) > 0 {
				userToken = itf.HardwareAddr.String()
				break
			}
		}
	}
	log.Printf("UserToken is %s\n", userToken)
}

func (manager *C4) Init() error {
	if enable, exist := common.Cfg.GetIntProperty("C4", "Enable"); exist {
		C4Enable = (enable != 0)
		if enable == 0 {
			return nil
		}
	}

	log.Println("Init C4.")
	initC4Config()
	RegisteRemoteConnManager(manager)
	manager.servers = &util.ListSelector{}

	c4HttpClient = new(http.Client)
	tlcfg := &tls.Config{}
	tlcfg.InsecureSkipVerify = true

	dial := func(n, addr string) (net.Conn, error) {
		if len(c4_cfg.Proxy) == 0 && !c4_cfg.UseSysDNS {
			remote := getAddressMapping(addr)
			return net.Dial(n, remote)
		}
		return net.Dial(n, addr)
	}
	tr := &http.Transport{
		DisableCompression:  true,
		MaxIdleConnsPerHost: int(c4_cfg.MaxConn * 2),
		TLSClientConfig:     tlcfg,
		Dial:                dial,
		Proxy: func(req *http.Request) (*url.URL, error) {
			if len(c4_cfg.Proxy) == 0 {
				return nil, nil
			}
			return url.Parse(c4_cfg.Proxy)
		},
		ResponseHeaderTimeout: time.Duration(c4_cfg.ReadTimeout + 1) * time.Second,
	}
	c4HttpClient.Transport = tr

	for i := uint32(0); i < c4_cfg.MaxConn; i++ {
		c4WriteCBChannels[i] = make(chan event.Event, 100)
		go writeCBLoop(i)
	}

	index := 0
	for {
		v, exist := common.Cfg.GetProperty("C4", "WorkerNode["+strconv.Itoa(index)+"]")
		if !exist || len(v) == 0 {
			break
		}
		if !strings.Contains(v, "://") {
			v = "http://" + v
		}
		if !strings.HasSuffix(v, "/") {
			v = v + "/"
		}
		manager.servers.Add(v)
		index = index + 1
		if strings.HasPrefix(v, "ws://") {
			initC4WebsocketChannel(v)
		}
		manager.loginC4(v)
	}
	if index == 0 {
		C4Enable = false
		return errors.New("No configed C4 server.")
	}
	return nil
}

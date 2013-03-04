package proxy

import (
	"bytes"
	"common"
	"encoding/binary"
	"errors"
	"event"
	"fmt"
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
	rangeStart             int
	originRangeHeader      string
	range_expected_pos     int
	rangeFetchWorkerNum    int
	proxyReqEvent          *event.HTTPRequestEvent
	proxyResEvent          *event.HTTPResponseEvent
	responsedChunks        map[int]*rangeChunk
	rangeFetchChunkPos     int
	injectRange            bool
	remoteHttpClientEnable bool
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
				//c4.closed = true
				//conn.Close()
				//c4.Close()
				//return io.EOF
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
		//log.Printf("Session[%d]Handle HTTP Response event.\n", conn.SessionID)
		res := ev.(*event.HTTPResponseEvent)
		httpres := res.ToResponse()
		contentRange := res.GetHeader("Content-Range")
		xhce := res.GetHeader("X-Snova-HCE")
		limit := 0
		if len(xhce) > 0 && len(contentRange) > 0 {
			c4.rangeFetchWorkerNum--
			if len(c4.originRangeHeader) == 0 {
				httpres.Header.Del("Content-Range")
			}
			startpos, endpos, length := util.ParseContentRangeHeaderValue(contentRange)
			log.Printf("Session[%d]Fetched range chunk[%d:%d-%d]", res.GetHash(), startpos, endpos, length)
			limit = length - 1
			if len(c4.originRangeHeader) > 0 {
				rs, re := util.ParseRangeHeaderValue(c4.originRangeHeader)
				if re > 0 {
					limit = re
				} 
				httpres.Header.Set("Content-Range", fmt.Sprintf("%d-%d/%d", rs, limit, limit+1))
			}
			if httpres.StatusCode < 300 {
				httpres.StatusCode = 200
				httpres.Status = ""
				httpres.ContentLength = int64(length - c4.rangeStart)
				if nil == c4.proxyResEvent {
					httpres.Write(conn.LocalRawConn)
					c4.proxyResEvent = res
					c4.responsedChunks = make(map[int]*rangeChunk)
					c4.rangeFetchChunkPos = endpos + 1
					c4.range_expected_pos = endpos + 1
				} else {
					chunk := &rangeChunk{}
					chunk.start = startpos
					chunk.content = &(res.Content)
					c4.responsedChunks[chunk.start] = chunk
				}
			} else {
				if httpres.StatusCode == 302 {
					location := res.GetHeader("Location")
					xrange := res.GetHeader("X-Range")
					log.Printf("Session[%d]X-Range=%s, Location= %s\n", res.GetHash(), xrange, location)
					if len(location) > 0 && len(xrange) > 0 && nil != c4.proxyReqEvent {
						c4.proxyReqEvent.Url = location
						c4.proxyReqEvent.SetHeader("Range", xrange)
						c4.rangeFetchWorkerNum++
						c4.offerRequestEvent(c4.proxyReqEvent)
						return nil

					}
				}
				httpres.Write(conn.LocalRawConn)
				return nil
			}
			for {
				if chunk, exist := c4.responsedChunks[c4.range_expected_pos]; exist {
					_, err := conn.LocalRawConn.Write(chunk.content.Bytes())
					delete(c4.responsedChunks, chunk.start)
					if nil != err {
						log.Printf("????????????????????????????%v\n", err)
						conn.LocalRawConn.Close()
						c4.Close()
						break
					} else {
						c4.range_expected_pos = c4.range_expected_pos + chunk.content.Len()
					}
					chunk.content = nil
				} else {
					log.Printf("Session[%d]Expected range chunk pos:%d", res.GetHash(), c4.range_expected_pos)
					break
				}
			}
			if endpos > 0 && endpos < limit {
				start := c4.rangeFetchChunkPos
				waterMark := (c4_cfg.FetchLimitSize + 1) * c4_cfg.ConcurrentRangeFetcher
				for !c4.closed && uint32(c4.rangeFetchWorkerNum) < c4_cfg.ConcurrentRangeFetcher && (start-c4.range_expected_pos) < int(waterMark) {
					begin := start
					if begin >= limit {
						break
					}
					end := start + int(c4_cfg.FetchLimitSize) - 1
					if end > limit {
						end = limit
					}
					start = end + 1
					c4.proxyReqEvent.SetHeader("Range", fmt.Sprintf("bytes=%d-%d", begin, end))
					c4.offerRequestEvent(c4.proxyReqEvent)
					log.Printf("Session[%d]Fetch range chunk:%s", res.GetHash(), fmt.Sprintf("bytes=%d-%d", begin, end))
					c4.rangeFetchWorkerNum++
					c4.rangeFetchChunkPos = start
				}
			}
		} else {
			httpres.Write(conn.LocalRawConn)
		}
	default:
		log.Printf("Unexpected event type:%d\n", ev.GetType())
	}
	return nil
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
		log.Printf("Session[%d]Request %s\n", req.GetHash(), util.GetURLString(req.RawReq, true))
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
		c4.remote_proxy_addr = remote_addr
		rangeHeader := req.GetHeader("Range")
		c4.originRangeHeader = rangeHeader
		if len(rangeHeader) > 0 {
			log.Printf("Session[%d]Request has range:%s\n", req.GetHash(), rangeHeader)
			startPos, endPos := util.ParseRangeHeaderValue(rangeHeader)
			if endPos == -1 {
				rangeHeader = ""
			} else {
				if endPos-startPos > int(c4_cfg.FetchLimitSize-1) {
					endPos = startPos + int(c4_cfg.FetchLimitSize-1)
					req.SetHeader("Range", fmt.Sprintf("bytes=%d-%d", startPos, endPos))
					req.SetHeader("X-Snova-HCE", "1")
					log.Printf("Session[%d]Inject range:%s\n", req.GetHash(), req.GetHeader("Range"))
					c4.rangeStart = startPos
					c4.proxyReqEvent = req
					c4.proxyResEvent = nil
					c4.rangeFetchWorkerNum++
				}
			}
		}
		if len(rangeHeader) == 0 {
			//inject range header
			if hostPatternMatched(c4_cfg.InjectRange, req.RawReq.Host) || c4.injectRange {
				req.SetHeader("X-Snova-HCE", "1")
				req.SetHeader("Range", "bytes=0-"+strconv.Itoa(int(c4_cfg.FetchLimitSize-1)))
				log.Printf("Session[%d]Inject range:%s\n", req.GetHash(), req.GetHeader("Range"))
				c4.proxyReqEvent = req
				c4.proxyResEvent = nil
				c4.rangeFetchWorkerNum++

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
	tr := &http.Transport{
		DisableCompression:  true,
		MaxIdleConnsPerHost: 20,
		Proxy: func(req *http.Request) (*url.URL, error) {
			if len(c4_cfg.Proxy) == 0 {
				return nil, nil
			}
			return url.Parse(c4_cfg.Proxy)
		},
	}
	c4HttpClient.Transport = tr

	for i := uint32(0); i < c4_cfg.MaxConn; i++ {
		c4WriteCBChannels[i] = make(chan event.Event, 10)
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

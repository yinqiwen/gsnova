package proxy

import (
	"bytes"
	"common"
	"encoding/binary"
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
	Compressor      uint32
	Encrypter       uint32
	UA              string
	ConnectionMode  string
	ReadTimeout     uint32
	MaxReadBytes    uint32
	MaxHTTPConn     uint32
	MaxWSConn       uint32
	WSConnKeepAlive uint32
	Proxy           string
}

var c4_cfg *C4Config
var c4HttpClient *http.Client

var c4SessionTable = make(map[uint32]*C4HttpConnection)
var c4SessionTableMutex sync.Mutex

type C4HttpConnection struct {
	sess               *SessionConnection
	tunnel_remote_addr string
	server             string
	closed             bool
	manager            *C4
}

func (c4 *C4HttpConnection) Close() error {
	if !c4.closed {
		closeEv := &event.SocketConnectionEvent{}
		closeEv.Status = event.TCP_CONN_CLOSED
		closeEv.SetHash(c4.sess.SessionID)
		offerRequestEvent(closeEv)
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

func (c4 *C4HttpConnection) offerRequestEvent(ev event.Event) {
	ev = wrapC4RequestEvent(ev)
	isWsServer := strings.HasPrefix(c4.server, "ws://")
	if isWsServer {
		wsOfferEvent(c4.server, ev)
		return
	}
	httpOfferEvent(c4.server, ev)
}

func (c4 *C4HttpConnection) handleTunnelResponse(conn *SessionConnection, ev event.Event) error {
	switch ev.GetType() {
	case event.EVENT_TCP_CONNECTION_TYPE:
		cev := ev.(*event.SocketConnectionEvent)
		if cev.Status == event.TCP_CONN_CLOSED {
			log.Printf("Session[%d]Remote %s connection closed, current proxy addr:%s\n", conn.SessionID, cev.Addr, c4.tunnel_remote_addr)
			if c4.tunnel_remote_addr == cev.Addr {
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
			log.Printf("############Not writed finished\n")
		}
	default:
		log.Printf("Unexpected event type:%d\n", ev.GetType())
	}
	return nil
}

func (c4 *C4HttpConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
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
		c4.tunnel_remote_addr = remote_addr
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
func (c4 *C4HttpConnection) GetConnectionManager() RemoteConnectionManager {
	return c4.manager
}

type C4 struct {
	servers *util.ListSelector
}

func setC4SessionTable(c4 *C4HttpConnection) {
	c4SessionTableMutex.Lock()
	c4SessionTable[c4.sess.SessionID] = conn
	c4SessionTableMutex.Unlock()
}

func removeC4SessionTable(c4 *C4HttpConnection) {
	c4SessionTableMutex.Lock()
	delete(c4SessionTable, c4.sess.SessionID)
	c4SessionTableMutex.Unlock()
}

func getC4Session(sid uint32) *C4HttpConnection {
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
	c4 := conn.(*C4HttpConnection)
}

func (manager *C4) loginC4(server string) {
	conn := &C4HttpConnection{}
	conn.manager = manager
	conn.server = server
	login := &event.UserLoginEvent{}
	login.User = userToken
	isWsServer := strings.HasPrefix(server, "ws://")
	if isWsServer {
		wsOfferEvent(server, login)
	} else {
		conn.requestEvent(login, false)
	}
}

func (manager *C4) GetRemoteConnection(ev event.Event, attrs map[string]string) (RemoteConnection, error) {
	conn := &C4HttpConnection{}
	conn.manager = manager
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

	c4_cfg.MaxReadBytes = 256 * 1024
	if period, exist := common.Cfg.GetIntProperty("C4", "MaxReadBytes"); exist {
		c4_cfg.MaxReadBytes = uint32(period)
	}

	c4_cfg.MaxWSConn = 5
	if num, exist := common.Cfg.GetIntProperty("C4", "MaxWSConn"); exist {
		c4_cfg.MaxWSConn = uint32(num)
	}

	c4_cfg.MaxHTTPConn = 5
	if num, exist := common.Cfg.GetIntProperty("C4", "MaxHTTPConn"); exist {
		c4_cfg.MaxHTTPConn = uint32(num)
	}

	c4_cfg.WSConnKeepAlive = 180
	if num, exist := common.Cfg.GetIntProperty("C4", "WSConnKeepAlive"); exist {
		c4_cfg.WSConnKeepAlive = uint32(num)
	}
	if tmp, exist := common.Cfg.GetProperty("C4", "Proxy"); exist {
		c4_cfg.Proxy = tmp
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

	index := 0
	for {
		v, exist := common.Cfg.GetProperty("C4", "WorkerNode["+strconv.Itoa(index)+"]")
		if !exist || len(v) == 0 {
			break
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

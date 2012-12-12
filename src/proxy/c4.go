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
	Compressor     uint32
	Encrypter      uint32
	UA             string
	ConnectionMode string
	ReadTimeout    uint32
	MaxReadBytes   uint32
}

var c4_cfg *C4Config

type C4HttpConnection struct {
	sess               *SessionConnection
	tunnelChannel      chan event.Event
	tunnel_remote_addr string
	server             string
	closed             bool
	manager            *C4
}

func (conn *C4HttpConnection) IsDisconnected() bool {
	return conn.closed
}

func (c4 *C4HttpConnection) Close() error {
	if !c4.closed {
		closeEv := &event.SocketConnectionEvent{}
		closeEv.Status = event.TCP_CONN_CLOSED
		closeEv.SetHash(c4.sess.SessionID)
		c4.offerRequestEvent(closeEv)
		c4.tunnelChannel <- nil
	}
	return nil
}

func (c4 *C4HttpConnection) requestEvent(ev event.Event, isPull bool) error {
	buf := new(bytes.Buffer)
	event.EncodeEvent(buf, ev)
	domain := c4.server
	pathstr := "invoke2"
	path := "/" + pathstr
	scheme := "http"
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		tmp := strings.SplitN(domain, "://", 2)
		scheme = tmp[0]
		domain = tmp[1]
	}
	rs := strings.SplitN(domain, "/", 2)
	if len(rs) == 2 {
		domain = rs[0]
		if strings.HasSuffix(rs[1], "/") {
			path = "/" + rs[1] + pathstr
		} else {
			path = "/" + rs[1] + pathstr
		}
	}
	req := &http.Request{
		Method:        "POST",
		URL:           &url.URL{Scheme: scheme, Host: domain, Path: path},
		Host:          domain,
		Header:        make(http.Header),
		Body:          ioutil.NopCloser(buf),
		ContentLength: int64(buf.Len()),
	}
	role := "pull"
	if !isPull {
		role = "push"
	}
	//log.Printf("%s\n", fmt.Sprintf("%s_%d_%d", role, c4_cfg.ReadTimeout, c4_cfg.MaxReadBytes))
	req.Header.Set("C4MiscInfo", fmt.Sprintf("%s_%d_%d", role, c4_cfg.ReadTimeout, c4_cfg.MaxReadBytes))

	req.Header.Set("UserToken", userToken)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/octet-stream")
	if len(c4_cfg.UA) > 0 {
		req.Header.Set("User-Agent", c4_cfg.UA)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	if resp.StatusCode != 200 {
		log.Printf("Session[%d]Role:%s unexpected response %s for %s\n", ev.GetHash(), role, resp.Status, c4.server)
		return fmt.Errorf("Invalid response with status:%d", resp.StatusCode)
	}
	// write http response response to conn

	handle_buffer := func(buffer *bytes.Buffer) bool {
		for buffer.Len() > 0 {
			err, evv := event.DecodeEvent(buffer)
			if nil == err {
				evv = event.ExtractEvent(evv)
				if nil != c4.handleTunnelResponse(c4.sess, evv) {
					return false
				}
			} else {
				log.Printf("Session[%d][ERROR]Decode event failed %v for role:%s", ev.GetHash(), err, role)
				return false
			}
		}
		return true
	}

	if resp.ContentLength > 0 {
		content := make([]byte, resp.ContentLength)
		n, err := io.ReadFull(resp.Body, content)
		if int64(n) != resp.ContentLength || nil != err {
			log.Printf("Failed to read data from body %d or %v", n, err)
			return fmt.Errorf("Read response failed %v", err)
		} else {
			//log.Printf("Session[%d]###### recv no TransferEncoding with %d  %v\n", ev.GetHash(), resp.ContentLength, resp.Header)
			if len(resp.Header.Get("C4LenHeader")) > 0 {
				buf = bytes.NewBuffer(content)
				for buf.Len() > 0 {
					var chunkLen int32
					binary.Read(buf, binary.BigEndian, &chunkLen)
					c := buf.Next(int(chunkLen))
					handle_buffer(bytes.NewBuffer(c))
				}
			} else {
				buf = bytes.NewBuffer(content)
				handle_buffer(buf)
			}
		}
	} else {
		if len(resp.TransferEncoding) > 0 {
			var chunkLen int32
			chunkLen = -1
			chunk := make([]byte, c4_cfg.MaxReadBytes+100)
			var buffer bytes.Buffer

			for !c4.closed{
				n, err := resp.Body.Read(chunk)
				if nil != err {
					break
				}
				buffer.Write(chunk[0:n])
				for {
					if chunkLen < 0 && buffer.Len() >= 4 {
						err = binary.Read(&buffer, binary.BigEndian, &chunkLen)
						if nil != err {
							log.Printf("#################%v\n", err)
							break
						}
					}
					if chunkLen >= 0 && buffer.Len() >= int(chunkLen) {
						content := buffer.Next(int(chunkLen))
						tmp := bytes.NewBuffer(content)
						if !handle_buffer(tmp) {
							goto R
						}
						buffer.Truncate(buffer.Len())
						chunkLen = -1
					} else {
						break
					}
				}
			}
		}
	}
R:
	buf.Reset()
	resp.Body.Close()
	return nil
}

func (c4 *C4HttpConnection) tunnel_write(conn *SessionConnection) {
    wait := 1 * time.Second
	for !c4.closed {
		select {
		case ev := <-c4.tunnelChannel:
			if nil == ev {
				c4.closed = true
				break
			}
			err := c4.requestEvent(ev, false)
			if nil != err{
			   time.Sleep(wait)
			   wait = 2 * wait
			   c4.tunnelChannel <- ev
			   continue
			}
			wait = 1 * time.Second
			if ev.GetType() == event.EVENT_TCP_CONNECTION_TYPE {
				tev := ev.(*event.SocketConnectionEvent)
				if tev.Status == event.TCP_CONN_CLOSED {
					c4.closed = true
					break
				}
			}
		}
	}
	atomic.AddInt32(&total_c4_routines, -1)
	log.Printf("Session[%d]close write tunnel.\n", conn.SessionID)
}

func (c4 *C4HttpConnection) tunnel_read(conn *SessionConnection) {
	wait := 1 * time.Second
	for !c4.closed {
		read := &event.SocketReadEvent{Timeout: c4_cfg.ReadTimeout, MaxRead: c4_cfg.MaxReadBytes}
		read.SetHash(conn.SessionID)
		err := c4.requestEvent(read, true)
		if nil == err {
			wait = 1 * time.Second
		} else {
			time.Sleep(wait)
			wait = 2 * wait
		}
	}
	atomic.AddInt32(&total_c4_routines, -1)
	log.Printf("Session[%d]close read tunnel.\n", conn.SessionID)
}

func (c4 *C4HttpConnection) offerRequestEvent(ev event.Event) {
	var encrypt event.EncryptEventV2
	encrypt.SetHash(ev.GetHash())
	encrypt.EncryptType = c4_cfg.Encrypter
	encrypt.Ev = ev
	c4.tunnelChannel <- &encrypt
}

func (c4 *C4HttpConnection) doCloseTunnel() {
	if nil != c4.tunnelChannel {
		closeEv := &event.SocketConnectionEvent{}
		closeEv.Status = event.TCP_CONN_CLOSED
		closeEv.SetHash(c4.sess.SessionID)
		c4.offerRequestEvent(closeEv)
	}
}

func (c4 *C4HttpConnection) handleTunnelResponse(conn *SessionConnection, ev event.Event) error {
	switch ev.GetType() {
	case event.EVENT_TCP_CONNECTION_TYPE:
		cev := ev.(*event.SocketConnectionEvent)
		if cev.Status == event.TCP_CONN_CLOSED {
			log.Printf("Session[%d]Remote %s connection closed, current proxy addr:%s\n", conn.SessionID, cev.Addr, c4.tunnel_remote_addr)
			if c4.tunnel_remote_addr == cev.Addr {
				conn.Close()
				c4.Close()
				return io.EOF
			}
		}
	case event.EVENT_TCP_CHUNK_TYPE:
		chunk := ev.(*event.TCPChunkEvent)
		log.Printf("Session[%d]Handle TCP chunk:%d with %d bytes\n", conn.SessionID, chunk.Sequence, len(chunk.Content))
		_, err := conn.LocalRawConn.Write(chunk.Content)
		if nil != err {
			log.Printf("[%d]Failed to write  data to local client:%v.\n", ev.GetHash(), err)
			conn.Close()
			c4.Close()
			return err
		}
	default:
		log.Printf("Unexpected event type:%d\n", ev.GetType())
	}
	return nil
}

func (c4 *C4HttpConnection) requestOverTunnel(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	if len(c4.server) == 0 {
		c4.server = c4.manager.servers.Select().(string)
	}
	if nil == c4.tunnelChannel {
		c4.tunnelChannel = make(chan event.Event, 10)
		go c4.tunnel_write(conn)
		go c4.tunnel_read(conn)
		atomic.AddInt32(&total_c4_routines, 2)
	}

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

		if req.RawReq.ContentLength > 0 {
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
		}
		req.RawReq.Body.Close()
	case event.HTTP_CHUNK_EVENT_TYPE:
		chunk := ev.(*event.HTTPChunkEvent)
		tcp_chunk := &event.TCPChunkEvent{Content: chunk.Content}
		tcp_chunk.SetHash(ev.GetHash())
		c4.offerRequestEvent(tcp_chunk)
		conn.State = STATE_RECV_HTTP_CHUNK
	}
	return nil, nil
}

func (c4 *C4HttpConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	c4.sess = conn
	return c4.requestOverTunnel(conn, ev)
}
func (c4 *C4HttpConnection) GetConnectionManager() RemoteConnectionManager {
	return c4.manager
}

type C4 struct {
	servers *util.ListSelector
}

func (manager *C4) RecycleRemoteConnection(conn RemoteConnection) {
	atomic.AddInt32(&total_c4_conn_num, -1)
}

func (manager *C4) loginC4(server string) {
	conn := &C4HttpConnection{}
	conn.manager = manager
	conn.server = server
	login := &event.UserLoginEvent{}
	login.User = userToken
	conn.requestEvent(login, false)
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
	if compress, exist := common.Cfg.GetProperty("C4", "Encrypter"); exist {
		if strings.EqualFold(compress, "None") {
			c4_cfg.Compressor = event.ENCRYPTER_NONE
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

	c4_cfg.ConnectionMode = MODE_HTTP

	logined = false
	if ifs, err := net.Interfaces(); nil == err{
	   for _, itf := range ifs{
	      if len(itf.HardwareAddr.String()) > 0{
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

	index := 0
	for {
		v, exist := common.Cfg.GetProperty("C4", "WorkerNode["+strconv.Itoa(index)+"]")
		if !exist || len(v) == 0 {
			break
		}
		manager.servers.Add(v)
		index = index + 1
		manager.loginC4(v)
	}
	if index == 0 {
		return errors.New("No configed C4 server.")
	}
	return nil
}
package remote

import (
	"bytes"
	"encoding/binary"
	"errors"
	"event"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var port = func() string {
	tmpport := os.Getenv("PORT")
	if tmpport == "" {
		tmpport = "5000"
	}

	return tmpport
}

type ProxySession struct {
	closed   bool
	id       uint32
	conn     net.Conn
	addr     string
	user     string
	recv_evs chan event.Event
}

func (serv *ProxySession) closeSession() {
	serv.closed = true
	if nil != serv.conn {
		log.Printf("[%d]Close session", serv.id)
		serv.conn.Close()
		serv.conn = nil
	}
}

func (serv *ProxySession) initConn(method, addr string) (err error) {
	if !strings.Contains(addr, ":") {
		if strings.EqualFold(method, "Connect") {
			addr = addr + ":443"
		} else {
			addr = addr + ":80"
		}
	}

	if nil == serv.conn || serv.addr != addr {
		if nil != serv.conn {
			serv.conn.Close()
			serv.conn = nil
		}
	}
	if nil != serv.conn {
		return nil
	}
	serv.addr = addr
	log.Printf("[%d]Connect remote:%s for method:%s", serv.id, addr, method)
	serv.conn, err = net.Dial("tcp", addr)
	if nil == err {
		//		log.Printf("[%d]Connect remote:%s success", serv.id, addr)
		go serv.readLoop()
		//}
		return nil
	} else {
		ev := &event.SocketConnectionEvent{Status: event.TCP_CONN_CLOSED}
		ev.Addr = addr
		ev.SetHash(serv.id)
		offerSendEvent(ev, serv.user)
		log.Printf("Failed to connect %s for reason:%v\n", addr, err)
	}

	return err
}

func (serv *ProxySession) readLoop() {
	if nil == serv.conn {
		return
	}
	remote := serv.addr
	var sequence uint32
	sequence = 0
	buf := make([]byte, 8*1024)
	for !serv.closed {
		if nil == serv.conn {
			//log.Println("[%d]Null conn.\n", serv.id)
			break
		}
		n, err := serv.conn.Read(buf)
		if n > 0 {
			content := make([]byte, n)
			copy(content, buf[0:n])
			ev := &event.TCPChunkEvent{Content: content, Sequence: sequence}
			ev.SetHash(serv.id)
			offerSendEvent(ev, serv.user)
			sequence = sequence + 1
			//log.Printf("[%d]Offer chunk with %d bytes\n", serv.id, n)
		}
		if nil != err {
			break
		}
	}
	ev := &event.SocketConnectionEvent{Status: event.TCP_CONN_CLOSED}
	ev.Addr = remote
	ev.SetHash(serv.id)
	offerSendEvent(ev, serv.user)
}

func recvEventLoop(user string) {
	evchan := recv_evs[user]
	tick := time.NewTicker(10 * time.Millisecond)
	for {
		select {
		case <-tick.C:
			continue
		case ev := <-evchan:
			if nil == ev {
				return
			}
			processRecvEvent(ev, user)
		}
	}
}

var proxySessionMap map[string]map[uint32]*ProxySession = make(map[string]map[uint32]*ProxySession)
var send_evs map[string][]chan event.Event = make(map[string][]chan event.Event)
var recv_evs map[string]chan event.Event = make(map[string]chan event.Event)
var rsock_conns map[string][]net.Conn = make(map[string][]net.Conn)
var rosck_write_routine_started = false

func processRecvEvent(ev event.Event, user string) {
	serv := getProxySession(user, ev.GetHash())
	ev = event.ExtractEvent(ev)
	switch ev.GetType() {
	case event.EVENT_USER_LOGIN_TYPE:
		req := ev.(*event.UserLoginEvent)
		closeProxyUser(req.User)
	case event.EVENT_TCP_CONNECTION_TYPE:
		req := ev.(*event.SocketConnectionEvent)
		if req.Status == event.TCP_CONN_CLOSED {
			deleteProxySession(user, ev.GetHash())
		}
	case event.HTTP_REQUEST_EVENT_TYPE:
		req := ev.(*event.HTTPRequestEvent)
		err := serv.initConn(req.Method, req.GetHeader("Host"))
		if nil != err {
			log.Printf("Failed to init conn for reason:%v\n", err)
		}
		if strings.EqualFold(req.Method, "Connect") {
			res := &event.TCPChunkEvent{}
			res.SetHash(ev.GetHash())
			if nil == serv.conn {
				res.Content = []byte("HTTP/1.1 503 ServiceUnavailable\r\n\r\n")
			} else {
				res.Content = []byte("HTTP/1.1 200 OK\r\n\r\n")
				//log.Printf("Return established.\n")
			}
			offerSendEvent(res, user)
		} else {
			if nil != serv.conn {
				err := req.Write(serv.conn)
				if nil != err {
					log.Printf("Failed to write http request %v\n", err)
					deleteProxySession(user, serv.id)
					return
				}

			} else {
				res := &event.TCPChunkEvent{}
				res.SetHash(ev.GetHash())
				res.Content = []byte("HTTP/1.1 503 ServiceUnavailable\r\n\r\n")
				offerSendEvent(res, serv.user)
			}
		}
	case event.EVENT_TCP_CHUNK_TYPE:
		if nil == serv.conn {
			//log.Printf("[%d]No session conn %d", ev.GetHash())
			deleteProxySession(serv.user, serv.id)
			return
		}
		chunk := ev.(*event.TCPChunkEvent)
		//.Printf("[%d]Chunk has %d", ev.GetHash(), len(chunk.Content))
		_, err := serv.conn.Write(chunk.Content)
		if nil != err {
			log.Printf("Failed to write chunk %v\n", err)
			serv.closeSession()
			return
		}
	}
}

func check_rsock_conn(user, server, addr string, pool_size int) {
	_, sock_exist := rsock_conns[user]
	if !sock_exist {
		rsock_conns[user] = make([]net.Conn, 0)

	}
	//conn_created := false
	conns := rsock_conns[user]
	for len(conns) < pool_size {
		conn, err := net.Dial("tcp", addr)
		if nil != err {
			log.Printf("Failed to connect %s\n", addr)
			return
		}
		log.Printf("Connect %s success\n", addr)
		conns = append(conns, conn)
		rsock_conns[user] = conns
		go rosck_read_routine(user, conn)
		_, ok := send_evs[user]
		if !ok {
			send_evs[user] = make([]chan event.Event, pool_size)
			for i, _ := range send_evs[user] {
				send_evs[user][i] = make(chan event.Event, 1024)
				go rosck_write_routine(user, i)
			}
		}
		accept := &event.RSocketAcceptedEvent{}
		accept.Server = server
		send_evs[user][len(conns) - 1] <- accept
	}
}

func rosck_read_routine(user string, conn net.Conn) {
	defer conn.Close()
	readEvent := func(buf *bytes.Buffer, length uint32) (error, event.Event, uint32) {
		if length == 0 {
			if buf.Len() > 4 {
				err := binary.Read(buf, binary.BigEndian, &length)
				if nil != err {
					log.Printf("read len error %v\n", err)
				}
			} else {
				return errors.New("Not sufficient space"), nil, 0
			}
		}
		if buf.Len() < int(length) {
			return errors.New("Not sufficient space"), nil, length
		}
		//log.Printf("Read Event length is %d\n", length)
		err, ev := event.DecodeEvent(buf)
		return err, ev, 0
	}
	decodebuf := &(bytes.Buffer{})
	tmpbuf := make([]byte, 8192)
	var lastEvLength uint32
	lastEvLength = 0
	for {
		n, err := conn.Read(tmpbuf)
		if nil != err {
			break
		}
		decodebuf.Write(tmpbuf[0:n])
		var ev event.Event
		for decodebuf.Len() > 0 {
			err, ev, lastEvLength = readEvent(decodebuf, lastEvLength)
			if nil != err {
				//log.Printf("Failed to read decode event: %v\n", err)
				break
			}
			processRecvEvent(ev, user)
		}
		if decodebuf.Len() == 0 {
			decodebuf.Reset()
		}
	}
	conns := rsock_conns[user]
	for i, it := range conns {
		if conn == it {
			conns = append(conns[:i], conns[i+1:]...)
			rsock_conns[user] = conns
			break
		}
	}
	//delete(rsock_conns, user)
}

func rosck_write_routine(user string, index int) {
	send_ev := send_evs[user][index]
	for {
		select {
		case ev := <-send_ev:
			conns, ok := rsock_conns[user]
			if !ok {
				continue
			}
			if nil == ev {
				return
			}
			var buf bytes.Buffer
			event.EncodeEvent(&buf, ev)
			length := uint32(buf.Len())
			conn := conns[int(ev.GetHash())%len(conns)]
			er := binary.Write(conn, binary.BigEndian, &length)
			if nil != er {
				log.Printf("write len error %v\n", er)
			}
			//log.Printf("Write Event length is %d\n", length)
			_, err := conn.Write(buf.Bytes())
			if nil != err {

			}
		}
	}
}

func closeProxyUser(name string) {
	sessions, exist := proxySessionMap[name]
	if exist {
		for _, sess := range sessions {
			sess.closeSession()
		}
		delete(proxySessionMap, name)
	}
	//	evchans, exist := send_evs[name]
	//	if exist {
	//		//close(evchan)
	//		//delete(send_evs, name)
	//	}
}

func sessionExist(name string, sessionID uint32) bool {
	_, exist := proxySessionMap[name]
	if exist {
		_, exist := proxySessionMap[name][sessionID]
		if exist {
			return true
		}
	}
	return false
}

func deleteProxySession(name string, sessionID uint32) {
	sessions, exist := proxySessionMap[name]
	if exist {
		sess, exist := proxySessionMap[name][sessionID]
		if exist {
			sess.closeSession()
			delete(sessions, sessionID)
		}
	}
}

func offerSendEvent(ev event.Event, user string) {
	switch ev.GetType() {
	case event.EVENT_TCP_CHUNK_TYPE:
		var compress event.CompressEventV2
		compress.SetHash(ev.GetHash())
		compress.Ev = ev
		compress.CompressType = event.COMPRESSOR_SNAPPY
		ev = &compress
	}
	var encrypt event.EncryptEventV2
	encrypt.SetHash(ev.GetHash())
	encrypt.EncryptType = event.ENCRYPTER_SE1
	encrypt.Ev = ev
	ev = &encrypt
	idx := int(ev.GetHash()) % len(send_evs[user])
	send_evs[user][idx] <- ev
}

func getProxySession(name string, sessionID uint32) *ProxySession {
	_, exist := proxySessionMap[name]
	if !exist {
		proxySessionMap[name] = make(map[uint32]*ProxySession)
	}
	sess, exist := proxySessionMap[name][sessionID]
	if !exist {
		sess = &ProxySession{closed: false, id: sessionID, user: name, recv_evs: make(chan event.Event, 4096)}
		proxySessionMap[name][sessionID] = sess
	}
	return sess
}

func InvokeCallback(w http.ResponseWriter, req *http.Request) {
	body, err := ioutil.ReadAll(req.Body)
	if nil != err {
	}
	user := req.Header.Get("UserToken")
	index := req.Header.Get("FetcherIndex")
	if len(index) == 0 {
		index = "0:1"
	}
	var idx, poolSize uint32
	fmt.Sscanf(index, "%d:%d", &idx, &poolSize)
	send_ev_array, exist := send_evs[user]
	if !exist {
		send_evs[user] = make([]chan event.Event, poolSize)
		for i, _ := range send_evs[user] {
			send_evs[user][i] = make(chan event.Event, 1024)
		}
	}
	recv_ev, ok := recv_evs[user]
	if !ok {
		recv_ev = make(chan event.Event, 4096)
		recv_evs[user] = recv_ev
		go recvEventLoop(user)
	}
	//log.Printf("#####%d\n",len(body))
	buf := bytes.NewBuffer(body)
	for {
		if buf.Len() == 0 {
			break
		}
		err, ev := event.DecodeEvent(buf)
		if nil != err {
			log.Printf("Decode event  error:%v", err)
			break
		}
		recv_ev <- ev
		//recv_evs[]
		//getProxySession(user, ev.GetHash()).recv_evs <- ev
	}
	var send_content bytes.Buffer
	start := time.Now().UnixNano()
	tick := time.NewTicker(10 * time.Millisecond)
	expectedData := true
	for expectedData {
		select {
		case <-tick.C:
			if time.Now().UnixNano()-start >= 100000*1000 {
				expectedData = false
				break
			}
			continue
		case ev := <-send_ev_array[idx]:
			if nil == ev {
				expectedData = false
				break
			}
			if sessionExist(user, ev.GetHash()) {
				event.EncodeEvent(&send_content, ev)
			}
			if send_content.Len() >= 16*1024 {
				expectedData = false
				break
			}
		}
	}
	//strconv.Itoa()
	w.Header().Set("Content-Length", strconv.Itoa(send_content.Len()))
	w.Write(send_content.Bytes())
}

// hello world, the web server
func IndexCallback(w http.ResponseWriter, req *http.Request) {
	io.WriteString(w, html)
}

func RSocketHeartBeat(w http.ResponseWriter, req *http.Request) {
	user := req.Header.Get("UserToken")
	server := req.Header.Get("RServer")
	addr := req.Header.Get("RServerAddress")
	pool_size_str := req.Header.Get("ConnectionPoolSize")
	pool_size, _ := strconv.Atoi(pool_size_str)
	check_rsock_conn(user, server, addr, pool_size)
	w.WriteHeader(200)
}

func LaunchC4HttpServer() {
	http.HandleFunc("/", IndexCallback)
	http.HandleFunc("/invoke", InvokeCallback)
	http.HandleFunc("/rsocket", RSocketHeartBeat)
	err := http.ListenAndServe(":"+port(), nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err.Error())
	}
}

const html = `
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN"
	"http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">

<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en" lang="en">
<head>
	<meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>
	<title>GSnova C4 Server</title>
</head>

<body>
  <div id="container">

    <h1><a href="http://github.com/yinqiwen/gsnova">GSnova</a>
      <span class="small">by <a href="http://twitter.com/yinqiwen">@yinqiwen</a></span></h1>

    <div class="description">
      Welcome to use GSnova C4 Server(V0.15.0)!
    </div>

	<h2>Code</h2>
    <p>You can clone the project with <a href="http://git-scm.com">Git</a>
      by running:
      <pre>$ git clone git://github.com/yinqiwen/gsnova.git</pre>
    </p>

    <div class="footer">
      get the source code on GitHub : <a href="http://github.com/yinqiwen/gsnova">yinqiwen/gsnova</a>
    </div>

  </div>
</body>
</html>
`

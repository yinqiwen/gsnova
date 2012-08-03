package remote

import (
	"bytes"
	"event"
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
	closed bool
	id     uint32
	conn   net.Conn
	addr   string
	//	send_sequence uint32
	//	recv_sequence uint32
	recv_evs chan event.Event
	//	recv_chunks   map[uint32]*event.SequentialChunkEvent
}

func (serv *ProxySession) closeSession() {
	serv.closed = true
	if nil != serv.conn {
		serv.conn.Close()
		serv.conn = nil
		//log.Printf("Close:%d\n", serv.id)
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
		//if !closeOldConn {
		go serv.readLoop()
		//}
		return nil
	} else {
		ev := &event.SocketConnectionEvent{Status: event.TCP_CONN_CLOSED}
		ev.Addr = addr
		ev.SetHash(serv.id)
		offerSendEvent(ev)
		log.Printf("Failed to connect %s for reason:%v\n", addr, err)
	}

	return err
}

func (serv *ProxySession) readLoop() {
	if nil == serv.conn {
		return
	}
	remote := serv.addr
	for !serv.closed {
		buf := make([]byte, 64*1024)
		if nil == serv.conn {
			//log.Println("[%d]Null conn.\n", serv.id)
			break
		}
		n, err := serv.conn.Read(buf)
		if n > 0 {
			ev := &event.TCPChunkEvent{Content: buf[0:n]}
			ev.SetHash(serv.id)
			offerSendEvent(ev)
			//log.Printf("Session[%d]Offer sequnece:%d\n", serv.id, serv.send_sequence)
			//serv.send_sequence = serv.send_sequence + 1
		}
		if nil != err {
			break
		}
	}
	ev := &event.SocketConnectionEvent{Status: event.TCP_CONN_CLOSED}
	ev.Addr = remote
	ev.SetHash(serv.id)
	offerSendEvent(ev)
}

func (serv *ProxySession) eventLoop() {
	tick := time.NewTicker(10 * time.Millisecond)
	for !serv.closed {
		select {
		case <-tick.C:
			continue
		case ev := <-serv.recv_evs:
			ev = event.ExtractEvent(ev)
			//log.Printf("[%d]Handle event %T", serv.id, ev.GetHash(), ev)
			switch ev.GetType() {
			case event.EVENT_TCP_CONNECTION_TYPE:
				req := ev.(*event.SocketConnectionEvent)
				if req.Status == event.TCP_CONN_CLOSED {

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
					offerSendEvent(res)
				} else {
					if nil != serv.conn {
						err := req.Write(serv.conn)
						if nil != err {
							log.Printf("Failed to write http request %v\n", err)
							serv.closeSession()
							return
						}

					} else {
						res := &event.TCPChunkEvent{}
						res.SetHash(ev.GetHash())
						res.Content = []byte("HTTP/1.1 503 ServiceUnavailable\r\n\r\n")
						offerSendEvent(res)
					}
				}
			case event.EVENT_TCP_CHUNK_TYPE:
				if nil == serv.conn {
					//log.Printf("[%d]No session conn %d", ev.GetHash())
					serv.closeSession()
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
	}
}

var proxySessionMap map[string]map[uint32]*ProxySession
var global_send_evs chan event.Event

func deleteProxySession(name string, sessionID uint32) {
	sessions, exist := proxySessionMap[name]
	if exist {
		delete(sessions, sessionID)
	}
}

func offerSendEvent(ev event.Event) {
	switch ev.GetType() {
	case event.EVENT_TCP_CONNECTION_TYPE, event.EVENT_TCP_CHUNK_TYPE:
		//		var compress event.CompressEventV2
		//		compress.SetHash(ev.GetHash())
		//		compress.Ev = ev
		//		compress.CompressType = event.COMPRESSOR_SNAPPY
		var encrypt event.EncryptEventV2
		encrypt.SetHash(ev.GetHash())
		encrypt.EncryptType = event.ENCRYPTER_SE1
		encrypt.Ev = ev
		ev = &encrypt
	}
	global_send_evs <- ev
}

func getProxySession(name string, sessionID uint32) *ProxySession {
	_, exist := proxySessionMap[name]
	if !exist {
		proxySessionMap[name] = make(map[uint32]*ProxySession)

	}
	sess, exist := proxySessionMap[name][sessionID]
	if !exist {
		sess = &ProxySession{closed: false, id: sessionID, recv_evs: make(chan event.Event, 4096)}
		proxySessionMap[name][sessionID] = sess
		go sess.eventLoop()
	}
	return sess
}

func InvokeCallback(w http.ResponseWriter, req *http.Request) {
	body, err := ioutil.ReadAll(req.Body)
	if nil != err {
	}
	user := req.Header.Get("UserToken")
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
		//log.Printf("Recv event %T", ev)
		getProxySession(user, ev.GetHash()).recv_evs <- ev
	}
	var send_content bytes.Buffer
	writeData := false
	start := time.Now().UnixNano()
	tick := time.NewTicker(10 * time.Millisecond)
	for {
		select {
		case <-tick.C:
			if time.Now().UnixNano()-start >= 50000*1000 {
				writeData = true
				break
			}
			continue
		case ev := <-global_send_evs:
			event.EncodeEvent(&send_content, ev)
			if send_content.Len() >= 64*1024 {
				writeData = true
				break
			}
		}
		if writeData {
			break
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

func LaunchC4HttpServer() {
	http.HandleFunc("/", IndexCallback)
	http.HandleFunc("/invoke", InvokeCallback)
	global_send_evs = make(chan event.Event, 4096)
	proxySessionMap = make(map[string]map[uint32]*ProxySession)
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

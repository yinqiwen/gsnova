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
	closed        bool
	id            uint32
	conn          net.Conn
	addr          string
	send_sequence uint32
	recv_sequence uint32
	recv_evs      chan event.Event
	recv_chunks   map[uint32]*event.SequentialChunkEvent
}

func (serv *ProxySession) closeSession() {
	serv.closed = true
	if nil != serv.conn {
		serv.conn.Close()
		serv.conn = nil
	}
}

func (serv *ProxySession) initConn(method, addr string) (err error) {
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
	if !strings.Contains(addr, ":") {
		if strings.EqualFold(method, "Connect") {
			addr = addr + ":443"
		} else {
			addr = addr + ":80"
		}
	}
	log.Printf("Connect remote:%s for method:%s", addr, method)
	serv.conn, err = net.Dial("tcp", addr)
	if nil == err {
		go serv.readLoop()
	}
	return
}

func (serv *ProxySession) readLoop() {
	if nil == serv.conn {
		return
	}
	for !serv.closed {
		buf := make([]byte, 64*1024)
		n, err := serv.conn.Read(buf)
		if n > 0 {
			ev := &event.SequentialChunkEvent{Sequence: serv.send_sequence, Content: buf[0:n]}
			ev.SetHash(serv.id)
			offerSendEvent(ev)
			log.Printf("Session[%d]Offer sequnece:%d\n", serv.id, serv.send_sequence)
			serv.send_sequence = serv.send_sequence + 1
		}
		if nil != err && err != io.EOF {
            log.Printf("Error:%v\n", err)
			break
		}
	}
	ev := &event.HTTPConnectionEvent{Status: event.HTTP_CONN_CLOSED}
	ev.SetHash(serv.id)
	offerSendEvent(ev)
}

func (serv *ProxySession) eventLoop() {
	for !serv.closed {
		select {
		case ev := <-serv.recv_evs:
			ev = event.ExtractEvent(ev)
			switch ev.GetType() {
			case event.HTTP_REQUEST_EVENT_TYPE:
				req := ev.(*event.HTTPRequestEvent)
				err := serv.initConn(req.Method, req.GetHeader("Host"))
				if nil != err {
					log.Printf("Failed to init conn for reason:%v\n", err)
				}
				if strings.EqualFold(req.Method, "Connect") {
					res := &event.HTTPResponseEvent{}
					if nil == serv.conn {
						res.Status = 503
					} else {
						res.Status = 200
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
						res := &event.HTTPResponseEvent{}
						res.Status = 503
						offerSendEvent(res)
					}
				}
			case event.EVENT_SEQUNCEIAL_CHUNK_TYPE:
				if nil == serv.conn {
					serv.closeSession()
					return
				}
				chunk := ev.(*event.SequentialChunkEvent)
				serv.recv_chunks[chunk.Sequence] = chunk
				for {
					sent_chunk, exist := serv.recv_chunks[serv.recv_sequence]
					if exist {
						_, err := serv.conn.Write(sent_chunk.Content)
						if nil != err {
							log.Printf("Failed to write chunk %v\n", err)
							serv.closeSession()
							return
						}
						serv.recv_sequence = serv.recv_sequence + 1
					} else {
						break
					}
				}
			}
		//default select
		default:
			time.Sleep(10 * time.Millisecond)
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
	case event.HTTP_RESPONSE_EVENT_TYPE, event.EVENT_SEQUNCEIAL_CHUNK_TYPE:
		var compress event.CompressEventV2
		compress.SetHash(ev.GetHash())
		compress.Ev = ev
		compress.CompressType = event.COMPRESSOR_SNAPPY
		var encrypt event.EncryptEventV2
		encrypt.SetHash(ev.GetHash())
		encrypt.EncryptType = event.ENCRYPTER_SE1
		encrypt.Ev = &compress
		ev = &encrypt
	default:
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
		sess.recv_chunks = make(map[uint32]*event.SequentialChunkEvent)
		go sess.eventLoop()
	}
	return sess
}

func InvokeCallback(w http.ResponseWriter, req *http.Request) {
	body, err := ioutil.ReadAll(req.Body)
	if nil != err {
	}
	user := req.Header.Get("UserToken")
	role := req.Header.Get("ClientActor")
	buf := bytes.NewBuffer(body)
	for {
		if buf.Len() == 0 {
			break
		}
		err, ev := event.DecodeEvent(buf)
		if nil != err {
			break
		}
		getProxySession(user, ev.GetHash()).recv_evs <- ev
	}
	var send_content bytes.Buffer
	writeData := false
	start := time.Now().Second()
	for {
		select {
		case ev := <-global_send_evs:
			event.EncodeEvent(&send_content, ev)
			if send_content.Len() >= 512*1024 {
				writeData = true
				break
			}
		default:
			writeData = true
			if strings.EqualFold(role, "assist") && send_content.Len() == 0 {
				writeData = false
				time.Sleep(10 * time.Millisecond)
				if time.Now().Second()-start >= 10 {
					writeData = true
				}
			}
			break
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

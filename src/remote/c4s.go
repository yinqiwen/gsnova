package remote

import (
	"bytes/buffer"
	"event"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
)

var port = func() string {
	tmpport := os.Getenv("PORT")
	if tmpport == "" {
		tmpport = "5000"
	}

	return tmpport
}

type ProxySession struct {
	serv     *UserProxyService
	conn     net.Conn
	addr     string
	recv_evs chan event.Event
}

type UserProxyService struct {
	name        string
	recv_evs    chan event.Event
	send_evs    chan event.Event
	session_map map[uint32]*ProxySession
}

func (serv *UserProxyService) eventLoop() {
	for {
		ev := <-serv.recv_evs
		ev = event.ExtractEvent(ev)
		switch ev.GetType() {
		case event.HTTP_REQUEST_EVENT_TYPE:
			req := ev.(*event.HTTPRequestEvent)
		case event.EVENT_SEQUNCEIAL_CHUNK_TYPE:
			chunk := ev.(*event.SequentialChunkEvent)

		}
	}
}

var userProxyServiceMap map[string]*UserProxyService

func getUserProxyService(name string) *UserProxyService {
	serv, exist := userProxyServiceMap[name]
	if !exist {
		serv = &UserProxyService{}
		serv.name = name
		serv.recv_evs = make(chan event.Event, 4096)
		serv.send_evs = make(chan event.Event, 4096)
		go serv.eventLoop()
	}
	return serv
}

func InvokeCallback(w http.ResponseWriter, req *http.Request) {
	body, err := ioutil.ReadAll(req.Body)
	if nil != err {
	}
	user := ""
	buf := bytes.NewBuffer(body)
	for {
		if buf.Len() == 0 {
			break
		}
		ev, err := event.DecodeEvent(buf)
		if nil != err {
			break
		}
		getUserProxyService(user).recv_evs <- ev
	}

}

// hello world, the web server
func IndexCallback(w http.ResponseWriter, req *http.Request) {
	io.WriteString(w, html)
}

func LaunchC4HttpServer() {
	http.HandleFunc("/", IndexCallback)
	http.HandleFunc("/invoke", InvokeCallback)
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

package remote

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/yinqiwen/gotoolkit/ots"
	"github.com/yinqiwen/gsnova/common/channel"
	httpChannel "github.com/yinqiwen/gsnova/common/channel/http"
	"github.com/yinqiwen/gsnova/common/channel/websocket"
	"github.com/yinqiwen/gsnova/common/logger"
)

// hello world, the web server
func indexCallback(w http.ResponseWriter, req *http.Request) {
	io.WriteString(w, strings.Replace(html, "${Version}", channel.Version, -1))
}

func statCallback(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(200)
	fmt.Fprintf(w, "Version:    %s\n", channel.Version)
	ots.Handle("stat", w)
}

func stackdumpCallback(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(200)
	ots.Handle("stackdump", w)
}

func startHTTPProxyServer(listenAddr string, cert, key string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", indexCallback)
	mux.HandleFunc("/stat", statCallback)
	mux.HandleFunc("/stackdump", stackdumpCallback)
	mux.HandleFunc("/ws", websocket.WebsocketInvoke)
	mux.HandleFunc("/http/pull", httpChannel.HTTPInvoke)
	mux.HandleFunc("/http/push", httpChannel.HTTPInvoke)
	mux.HandleFunc("/http/test", httpChannel.HttpTest)

	logger.Info("Listen on HTTP address:%s", listenAddr)
	var err error
	if len(cert) == 0 {
		err = http.ListenAndServe(listenAddr, mux)
	} else {
		err = http.ListenAndServeTLS(listenAddr, cert, key, mux)
	}

	if nil != err {
		logger.Error("Listen HTTP server error:%v", err)
	}
}

const html = `
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN"
	"http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">

<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en" lang="en">
<head>
	<meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>
	<title>GSnova PAAS Server</title>
</head>

<body>
  <div id="container">

    <h1><a href="http://github.com/yinqiwen/gsnova">GSnova</a>
      <span class="small">by <a href="http://twitter.com/yinqiwen">@yinqiwen</a></span></h1>

    <div class="description">
      Welcome to use GSnova HTTP/WebSocket Server ${Version}!
    </div>

	<h2>Code</h2>
    <p>You can clone the project with <a href="http://git-scm.com">Git</a>
      by running:
      <pre>$ git clone https://github.com/yinqiwen/gsnova.git</pre>
    </p>

    <div class="footer">
      get the source code on GitHub : <a href="http://github.com/yinqiwen/gsnova">yinqiwen/gsnova</a>
    </div>

  </div>
</body>
</html>
`

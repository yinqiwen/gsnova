package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/yinqiwen/gotoolkit/ots"
	"github.com/yinqiwen/gsnova/remote"
)

// hello world, the web server
func indexCallback(w http.ResponseWriter, req *http.Request) {
	io.WriteString(w, strings.Replace(html, "${Version}", remote.Version, -1))
}

func statCallback(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(200)
	fmt.Fprintf(w, "Version:    %s\n", remote.Version)
	fmt.Fprintf(w, "NumSession: %d\n", remote.GetSessionTableSize())
	ots.Handle("stat", w)
}

func stackdumpCallback(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(200)
	ots.Handle("stackdump", w)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("OPENSHIFT_GO_PORT")
	}
	if port == "" {
		port = os.Getenv("VCAP_APP_PORT")
	}
	if port == "" {
		port = "8080"
	}
	host := os.Getenv("OPENSHIFT_GO_IP")

	mux := http.NewServeMux()
	mux.HandleFunc("/", indexCallback)
	mux.HandleFunc("/stat", statCallback)
	mux.HandleFunc("/stackdump", stackdumpCallback)
	mux.HandleFunc("/ws", websocketInvoke)
	mux.HandleFunc("/http/pull", httpInvoke)
	mux.HandleFunc("/http/push", httpInvoke)

	log.Println("Listening on " + host + ":" + port)
	err := http.ListenAndServe(host+":"+port, mux)
	if nil != err {
		fmt.Printf("Listen server error:%v", err)
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
      Welcome to use GSnova PAAS Server ${Version}!
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

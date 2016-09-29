// +build appengine
package gae

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/remote"

	"appengine"
	"appengine/urlfetch"
)

func fetch(context appengine.Context, ev *event.HTTPRequestEvent) event.Event {
	errorResponse := new(event.NotifyEvent)
	errorResponse.SetId(ev.GetId())
	req, err := ev.ToRequest("")

	if nil != err {
		errorResponse.Code = event.ErrInvalidHttpRequest
		errorResponse.Reason = fmt.Sprintf("Invalid fetch url:%s with err:%v", ev.URL, err)
		return errorResponse
	}
	var t urlfetch.Transport
	t.Context = context
	t.Deadline, _ = time.ParseDuration("10s")
	t.AllowInvalidServerCertificate = true
	retryCount := 2
	for retryCount > 0 {
		resp, err := t.RoundTrip(req)
		if err == nil {
			res := event.NewHTTPResponseEvent(resp)
			for nil != resp.Body {
				buffer := make([]byte, 8192)
				n, er := resp.Body.Read(buffer)
				if nil != er {
					context.Errorf("Failed to read body for reason:%v", er)
					break
				}
				res.Content = append(res.Content, buffer[0:n]...)
			}
			if resp.ContentLength != int64(len(res.Content)) {
				context.Errorf("Failed to read body %d %d", resp.ContentLength, len(res.Content))
			}
			context.Errorf("%v %d %d", resp.Header.Get("Content-Length"), resp.ContentLength, len(res.Content))
			return res
		}
		context.Errorf("Failed to fetch URL[%s] for reason:%v", ev.URL, err)
		retryCount--
		if strings.EqualFold(req.Method, "GET") && strings.Contains(err.Error(), "RESPONSE_TOO_LARGE") {
			errorResponse.Code = event.ErrTooLargeResponse
			return errorResponse
		}
	}
	errorResponse.Code = event.ErrRemoteProxyTimeout
	errorResponse.Reason = fmt.Sprintf("Fetch timeout for url:%s", ev.URL)
	return errorResponse

}

func statCallback(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(200)
	fmt.Fprintf(w, "Version:    %s\n", remote.Version)
}

func httpInvoke(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, r.ContentLength)
	r.Body.Read(b)
	buf := bytes.NewBuffer(b)
	ctx := appengine.NewContext(r)

	var cryptoContext event.CryptoContext
	err, ev := event.DecryptEvent(buf, &cryptoContext)
	if nil != err {
		ctx.Errorf("Decode auth event failed:%v", err)
		return
	}
	//var iv uint64
	if auth, ok := ev.(*event.AuthEvent); ok {
		if !remote.ServerConf.VerifyUser(auth.User) {
			return
		}
		cryptoContext.DecryptIV = auth.IV
		cryptoContext.EncryptIV = auth.IV
		cryptoContext.Method = auth.EncryptMethod
	} else {
		ctx.Errorf("Expected auth event, but got %T", ev)
		return
	}

	err, ev = event.DecryptEvent(buf, &cryptoContext)
	if nil != err {
		ctx.Errorf("Decode http request event failed:%v", err)
		return
	}
	if req, ok := ev.(*event.HTTPRequestEvent); ok {
		res := fetch(ctx, req)
		var resbuf bytes.Buffer
		event.EncryptEvent(&resbuf, res, &cryptoContext)
		headers := w.Header()
		headers.Add("Content-Type", "application/octet-stream")
		headers.Add("Content-Length", strconv.Itoa(resbuf.Len()))
		w.WriteHeader(http.StatusOK)
		w.Write(resbuf.Bytes())
	} else {
		ctx.Errorf("Expected http request event, but got %T", ev)
		return
	}
}

func index(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, strings.Replace(html, "${Version}", remote.Version, -1))
}

func init() {
	http.HandleFunc("/invoke", httpInvoke)
	http.HandleFunc("/stat", statCallback)
	http.HandleFunc("/", index)
}

const html = `
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN"
	"http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">

<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en" lang="en">
<head>
	<meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>
	<title>GSnova GAE Server</title>
</head>

<body>
  <div id="container">

    <h1><a href="http://github.com/yinqiwen/gsnova">GSnova</a>
      <span class="small">by <a href="http://twitter.com/yinqiwen">@yinqiwen</a></span></h1>

    <div class="description">
      Welcome to use GSnova GAE Server ${Version}!
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

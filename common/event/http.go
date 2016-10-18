package event

import (
	"bytes"
	"fmt"
	//"fmt"
	//"net"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type HTTPMessageEvent struct {
	EventHeader
	Headers http.Header
	Content []byte
}

func (msg *HTTPMessageEvent) IsContentFull() bool {
	length := msg.GetContentLength()
	if len(msg.Content) < length {
		return false
	}
	return true
}

func (msg *HTTPMessageEvent) GetContentLength() int {
	he := msg.Headers.Get("Content-Length")
	if len(he) == 0 {
		return 0
	}
	v, err := strconv.Atoi(he)
	if nil != err {
		return 0
	}
	return v
}

func (msg *HTTPMessageEvent) IsKeepAlive() bool {
	he := msg.Headers.Get("Connection")
	if len(he) == 0 {
		return false
	}
	if strings.EqualFold(he, "keep-alive") {
		return true
	}
	return false
}

func (msg *HTTPMessageEvent) DoEncode(buffer *bytes.Buffer) {
	for hn, hvs := range msg.Headers {
		for _, hv := range hvs {
			EncodeBytesValue(buffer, []byte(hn))
			EncodeBytesValue(buffer, []byte(hv))
		}
	}
	EncodeBytesValue(buffer, []byte("\r\n"))
	EncodeBytesValue(buffer, msg.Content)
}

func (msg *HTTPMessageEvent) DoDecode(buffer *bytes.Buffer) error {
	msg.Headers = make(http.Header)
	for {
		headerName, err := DecodeBytesValue(buffer)
		if err != nil {
			return err
		}
		if string(headerName) == "\r\n" {
			break
		}
		headerValue, err := DecodeBytesValue(buffer)
		if err != nil {
			return err
		}
		msg.Headers.Add(string(headerName), string(headerValue))
	}
	var err error
	msg.Content, err = DecodeBytesValue(buffer)
	if err != nil {
		return err
	}
	return nil
}

type HTTPRequestEvent struct {
	HTTPMessageEvent
	Method string
	URL    string
}

func (req *HTTPRequestEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, req.URL)
	EncodeStringValue(buffer, req.Method)
	req.DoEncode(buffer)
}
func (req *HTTPRequestEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.URL, err = DecodeStringValue(buffer)
	if err != nil {
		return
	}
	req.Method, err = DecodeStringValue(buffer)
	if err != nil {
		return
	}
	return req.DoDecode(buffer)
}

func (req *HTTPRequestEvent) GetHost() string {
	host := req.Headers.Get("Host")
	return host
}

func (req *HTTPRequestEvent) ToRequest(schema string) (*http.Request, error) {
	if !strings.Contains(req.URL, "://") {
		req.URL = schema + "://" + req.Headers.Get("Host") + req.URL
	}
	httpReq, err := http.NewRequest(req.Method, req.URL, NewHTTPBody(req.GetContentLength(), req.Content))
	if nil != err {
		return nil, err
	}
	httpReq.Header = req.Headers
	return httpReq, nil
}

func (req *HTTPRequestEvent) HTTPEncode() []byte {
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "%s %s HTTP/1.1\r\n", req.Method, req.URL)
	for hn, hvs := range req.Headers {
		for _, hv := range hvs {
			fmt.Fprintf(&buffer, "%s: %s\r\n", hn, hv)
		}
	}
	fmt.Fprintf(&buffer, "\r\n")
	if len(req.Content) > 0 {
		buffer.Write(req.Content)
	}
	return buffer.Bytes()
}

type HTTPResponseEvent struct {
	HTTPMessageEvent
	StatusCode uint32
}

func (res *HTTPResponseEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt64Value(buffer, uint64(res.StatusCode))
	res.DoEncode(buffer)
}
func (res *HTTPResponseEvent) Decode(buffer *bytes.Buffer) (err error) {
	res.StatusCode, err = DecodeUInt32Value(buffer)
	if err != nil {
		return
	}
	return res.DoDecode(buffer)
}

// func (res *HTTPResponseEvent) ToResponse() *http.Response {
// 	raw := new(http.Response)
// 	raw.Proto = "HTTP"
// 	raw.ProtoMajor = 1
// 	raw.ProtoMinor = 0
// 	//raw.Close = true
// 	raw.ContentLength = int64(res.Content.Len())
// 	raw.Header = make(http.Header)
// 	raw.StatusCode = int(res.Status)
// 	res.SetHeader("Content-Length", strconv.Itoa(res.Content.Len()))
// 	for i := 0; i < len(res.Headers); i++ {
// 		header := res.Headers[i]
// 		if strings.EqualFold(header.Name, "Set-Cookie") || strings.EqualFold(header.Name, "Set-Cookie2") {
// 			tmp := strings.Split(header.Value, ",")
// 			if len(tmp) > 1 {
// 				var vlist list.List
// 				for _, v := range tmp {
// 					if (!strings.Contains(v, "=") || strings.Index(v, "=") > strings.Index(v, ";")) && vlist.Len() > 0 {
// 						v = vlist.Back().Value.(string) + "," + v
// 						vlist.Remove(vlist.Back())
// 						vlist.PushBack(v)
// 						//headerValues.add(headerValues.removeLast() + "," + v);
// 					} else {
// 						vlist.PushBack(v)
// 					}
// 				}
// 				e := vlist.Front()
// 				for {
// 					if e == nil {
// 						break
// 					}
// 					raw.Header.Add(header.Name, e.Value.(string))
// 					e = e.Next()
// 				}
// 			} else {
// 				raw.Header.Add(header.Name, header.Value)
// 			}
// 		} else {
// 			raw.Header.Add(header.Name, header.Value)
// 		}
// 	}
// 	if raw.ContentLength > 0 {
// 		raw.Body = &util.BufferCloseWrapper{&res.Content}
// 	}
// 	return raw
// }

func (res *HTTPResponseEvent) ToResponse(body bool) *http.Response {
	raw := new(http.Response)
	raw.Header = res.Headers
	raw.ProtoMajor = 1
	raw.ProtoMinor = 1
	raw.StatusCode = int(res.StatusCode)
	raw.Status = http.StatusText(raw.StatusCode)
	raw.TransferEncoding = res.Headers["TransferEncoding"]
	if body {
		raw.Body = NewHTTPBody(res.GetContentLength(), res.Content)
	}
	return raw
}

func (res *HTTPResponseEvent) Write(w io.Writer) (int, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "HTTP/1.1 %d %s\r\n", res.StatusCode, http.StatusText(int(res.StatusCode)))
	res.Headers.Write(&buf)
	buf.Write([]byte("\r\n"))
	//log.Printf("[%d]### %d  %s", res.GetId(), len(res.Content), string(buf.Bytes()))
	if len(res.Content) > 0 {
		buf.Write(res.Content)
		//log.Printf("###  %s", string(res.Content))
	}
	return w.Write(buf.Bytes())
}

func NewHTTPResponseEvent(res *http.Response) *HTTPResponseEvent {
	ev := new(HTTPResponseEvent)
	ev.Headers = res.Header
	ev.StatusCode = uint32(res.StatusCode)
	if res.ContentLength >= 0 {
		ev.Headers.Set("Content-Length", fmt.Sprintf("%d", res.ContentLength))
	}
	if res.ContentLength != 0 {
		readLen := 8192
		if res.ContentLength > 0 && res.ContentLength < int64(readLen) {
			readLen = int(res.ContentLength)
		}
		body := make([]byte, readLen)
		n, _ := res.Body.Read(body)
		ev.Content = body[0:n]
	}
	return ev
}

func NewHTTPRequestEvent(req *http.Request) *HTTPRequestEvent {
	ev := new(HTTPRequestEvent)
	ev.Headers = req.Header
	ev.Method = req.Method
	ev.URL = req.URL.String()
	if strings.HasPrefix(ev.URL, "http://") {
		ev.URL = ev.URL[7:]
		ev.URL = ev.URL[len(req.Host):]
	}
	ev.Headers.Set("Host", req.Host)
	if len(req.TransferEncoding) > 0 && req.TransferEncoding[0] == "chunked" {
		ev.Headers.Add("Transfer-Encoding", "chunked")
	}
	if req.ContentLength != 0 {
		readLen := 8092
		if req.ContentLength > 0 && req.ContentLength < int64(readLen) {
			readLen = int(req.ContentLength)
		}
		body := make([]byte, readLen)
		n, _ := req.Body.Read(body)
		ev.Content = body[0:n]
	}
	return ev
}

// func (res *HTTPResponseEvent) FromResponse(raw *http.Response) {
// 	res.rawRes = raw
// 	res.Status = uint32(raw.StatusCode)
// 	for key, values := range raw.Header {
// 		for _, value := range values {
// 			res.AddHeader(key, value)
// 		}
// 	}
// }

type HTTPBody struct {
	length int
	readed int
	body   *bytes.Buffer
	c      chan []byte
}

func (b *HTTPBody) Close() error {
	if b.c != nil {
		close(b.c)
	}
	b.body.Reset()
	return nil
}

func (b *HTTPBody) Add(content []byte) {
	if nil != b.c {
		b.c <- content
	}
}

func (b *HTTPBody) Read(p []byte) (n int, err error) {
	if b.readed == b.length {
		return 0, io.EOF
	}
	if b.body.Len() == 0 {
		content := <-b.c
		b.body = bytes.NewBuffer(content)
	}
	n, err = b.body.Read(p)
	if b.body.Len() == 0 {
		b.body.Reset()
	}
	if n > 0 {
		b.readed += n
	}
	return
}

func NewHTTPBody(length int, body []byte) *HTTPBody {
	b := new(HTTPBody)
	b.length = length
	b.body = bytes.NewBuffer(body)
	b.readed = 0
	if length > 0 && len(body) < length {
		b.c = make(chan []byte, 10)
	}
	return b
}

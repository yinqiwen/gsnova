package event

import (
	"bytes"
	"container/list"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"util"
)

type HTTPConnectionEvent struct {
	Status uint64
	EventHeader
}

func (req *HTTPConnectionEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt64Value(buffer, req.Status)
}
func (req *HTTPConnectionEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.Status, err = DecodeUInt64Value(buffer)
	if err != nil {
		return
	}
	return nil
}

func (req *HTTPConnectionEvent) GetType() uint32 {
	return HTTP_CONNECTION_EVENT_TYPE
}
func (req *HTTPConnectionEvent) GetVersion() uint32 {
	return 1
}

type HTTPErrorEvent struct {
	Error int64
	Cause string
	EventHeader
}

func (req *HTTPErrorEvent) Encode(buffer *bytes.Buffer) {
	EncodeInt64Value(buffer, req.Error)
	EncodeStringValue(buffer, req.Cause)
}
func (req *HTTPErrorEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.Error, err = DecodeInt64Value(buffer)
	if err != nil {
		return
	}
	req.Cause, err = DecodeStringValue(buffer)
	if err != nil {
		return
	}
	return nil
}

func (req *HTTPErrorEvent) GetType() uint32 {
	return HTTP_ERROR_EVENT_TYPE
}
func (req *HTTPErrorEvent) GetVersion() uint32 {
	return 1
}

type HTTPMessageEvent struct {
	Headers []*util.NameValuePair
	Content bytes.Buffer
	EventHeader
}

func (msg *HTTPMessageEvent) getHeaderEntry(name string) *util.NameValuePair {
	for i := 0; i < len(msg.Headers); i++ {
		header := msg.Headers[i]
		if strings.EqualFold(header.Name, name) {
			return header
		}
	}
	return nil
}

func (msg *HTTPMessageEvent) AddHeader(name, value string) {
	pair := util.NameValuePair{name, value}
	msg.Headers = append(msg.Headers, &pair)
}

func (msg *HTTPMessageEvent) SetHeader(name, value string) {
	he := msg.getHeaderEntry(name)
	if nil != he {
		he.Value = value
	} else {
		msg.AddHeader(name, value)
	}
}

func (msg *HTTPMessageEvent) GetHeader(name string) string {
	he := msg.getHeaderEntry(name)
	if nil != he {
		return he.Value
	}
	return ""
}

func (msg *HTTPMessageEvent) RemoveHeader(name string) {
	for i := 0; i < len(msg.Headers); i++ {
		header := msg.Headers[i]
		if strings.EqualFold(header.Name, name) {
			msg.Headers = append(msg.Headers[:i], msg.Headers[i+1:]...)
			i--
		}
	}
}

func (msg *HTTPMessageEvent) IsContentFull() bool {
	length := msg.GetContentLength()
	if msg.Content.Len() < length {
		return false
	}
	return true
}

func (msg *HTTPMessageEvent) GetContentLength() int {
	he := msg.getHeaderEntry("Content-Length")
	if nil == he {
		return 0
	}
	v, err := strconv.Atoi(he.Value)
	if nil != err {
		return 0
	}
	return v
}

func (msg *HTTPMessageEvent) IsKeepAlive() bool {
	he := msg.getHeaderEntry("Connection")
	if nil == he {
		return false
	}
	if strings.EqualFold(he.Value, "keep-alive") {
		return true
	}
	return false
}

func (msg *HTTPMessageEvent) DoEncode(buffer *bytes.Buffer) {
	EncodeUInt64Value(buffer, uint64(len(msg.Headers)))
	for _, header := range msg.Headers {
		EncodeBytesValue(buffer, []byte(header.Name))
		EncodeBytesValue(buffer, []byte(header.Value))
	}
	b := msg.Content.Bytes()
	EncodeBytesValue(buffer, b)
}

func (msg *HTTPMessageEvent) DoDecode(buffer *bytes.Buffer) error {
	length, err := DecodeUInt32Value(buffer)
	if err != nil {
		return err
	}
	for i := 0; i < int(length); i++ {
		headerName, err := DecodeBytesValue(buffer)
		if err != nil {
			return err
		}
		headerValue, err := DecodeBytesValue(buffer)
		if err != nil {
			return err
		}
		pair := util.NameValuePair{string(headerName), string(headerValue)}
		msg.Headers = append(msg.Headers, &pair)
	}
	err = DecodeByteBufferValue(buffer, &msg.Content)
	if err != nil {
		return err
	}
	//msg.Content.Write(b)
	return nil
}

type HTTPChunkEvent struct {
	Content []byte
	EventHeader
}

func (chunk *HTTPChunkEvent) Encode(buffer *bytes.Buffer) {
	EncodeBytesValue(buffer, chunk.Content)
}
func (chunk *HTTPChunkEvent) Decode(buffer *bytes.Buffer) (err error) {
	chunk.Content, err = DecodeBytesValue(buffer)
	return
}

func (req *HTTPChunkEvent) GetType() uint32 {
	return HTTP_CHUNK_EVENT_TYPE
}
func (req *HTTPChunkEvent) GetVersion() uint32 {
	return 1
}

type HTTPRequestEvent struct {
	HTTPMessageEvent
	Method string
	Url    string
	RawReq *http.Request
}

func (req *HTTPRequestEvent) DeepClone() *HTTPRequestEvent {
	ret := new(HTTPRequestEvent)
	ret.Method = req.Method
	ret.Url = req.Url
	ret.RawReq = req.RawReq
	ret.HTTPMessageEvent.Headers = make([]*util.NameValuePair, len(req.HTTPMessageEvent.Headers))
	for i, v := range req.HTTPMessageEvent.Headers {
		nv := new(util.NameValuePair)
		nv.Name = v.Name
		nv.Value = v.Value
		ret.HTTPMessageEvent.Headers[i] = nv
	}
	ret.HTTPMessageEvent.Content.Write(req.HTTPMessageEvent.Content.Bytes())
	return ret
}

func (req *HTTPRequestEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, req.Url)
	EncodeStringValue(buffer, req.Method)
	req.DoEncode(buffer)
}
func (req *HTTPRequestEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.Url, err = DecodeStringValue(buffer)
	if err != nil {
		return
	}
	req.Method, err = DecodeStringValue(buffer)
	if err != nil {
		return
	}
	return req.DoDecode(buffer)
}

func (req *HTTPRequestEvent) ToRequest() *http.Request {
	raw, err := http.NewRequest(req.Method, req.Url, &(req.Content))
	if err != nil {
		return nil
	}
	for i := 0; i < len(req.Headers); i++ {
		header := req.Headers[i]
		raw.Header.Add(header.Name, header.Value)
	}
	return raw
}

func (req *HTTPRequestEvent) Write(conn net.Conn) error {
	var buf bytes.Buffer
	buf.Write([]byte(fmt.Sprintf("%s %s HTTP/1.1\r\n", req.Method, req.Url)))
	for i := 0; i < len(req.Headers); i++ {
		header := req.Headers[i]
		buf.Write([]byte(fmt.Sprintf("%s:%s\r\n", header.Name, header.Value)))
	}
	buf.Write([]byte("\r\n"))
	buf.Write(req.Content.Bytes())
	_, err := conn.Write(buf.Bytes())
	return err
}

func (req *HTTPRequestEvent) FromRequest(raw *http.Request) {
	req.RawReq = raw
	req.Url = raw.RequestURI
	req.Method = raw.Method
	for key, values := range raw.Header {
		for _, value := range values {
			req.AddHeader(key, value)
		}
	}
	//	if !strings.Contains(req.Url, raw.Host) {
	//	   scheme := "http://"
	//	   if req.IsHttps{
	//	      scheme := "https://"
	//	   }
	//	   req.Url = scheme + raw.Host + raw.RequestURI
	//	}
	req.AddHeader("Host", raw.Host)
}

func (req *HTTPRequestEvent) GetType() uint32 {
	return HTTP_REQUEST_EVENT_TYPE
}
func (req *HTTPRequestEvent) GetVersion() uint32 {
	return 1
}

type HTTPResponseEvent struct {
	HTTPMessageEvent
	Status uint32
	rawRes *http.Response
}

func (res *HTTPResponseEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt64Value(buffer, uint64(res.Status))
	res.DoEncode(buffer)
}
func (res *HTTPResponseEvent) Decode(buffer *bytes.Buffer) (err error) {
	res.Status, err = DecodeUInt32Value(buffer)
	if err != nil {
		return
	}
	return res.DoDecode(buffer)
}

func (res *HTTPResponseEvent) ToResponse() *http.Response {
	raw := new(http.Response)
	raw.Proto = "HTTP"
	raw.ProtoMajor = 1
	raw.ProtoMinor = 1
	//raw.Close = true
	raw.ContentLength = int64(res.Content.Len())
	raw.Header = make(http.Header)
	raw.StatusCode = int(res.Status)
	res.SetHeader("Content-Length", strconv.Itoa(res.Content.Len()))
	for i := 0; i < len(res.Headers); i++ {
		header := res.Headers[i]
		if strings.EqualFold(header.Name, "Set-Cookie") || strings.EqualFold(header.Name, "Set-Cookie2") {
			tmp := strings.Split(header.Value, ",")
			if len(tmp) > 1 {
				var vlist list.List
				for _, v := range tmp {
					if (!strings.Contains(v, "=") || strings.Index(v, "=") > strings.Index(v, ";")) && vlist.Len() > 0 {
						v = vlist.Back().Value.(string) + "," + v
						vlist.Remove(vlist.Back())
						vlist.PushBack(v)
						//headerValues.add(headerValues.removeLast() + "," + v);
					} else {
						vlist.PushBack(v)
					}
				}
				e := vlist.Front()
				for {
					if e == nil {
						break
					}
					raw.Header.Add(header.Name, e.Value.(string))
					e = e.Next()
				}
			} else {
				raw.Header.Add(header.Name, header.Value)
			}
		} else {
			raw.Header.Add(header.Name, header.Value)
		}
	}
	if raw.ContentLength > 0 {
		raw.Body = ioutil.NopCloser(&res.Content)
	}
	return raw
}

//func (res *HTTPResponseEvent) ToBuffer(buf *bytes.Buffer) *http.Response {
//	raw := new(http.Response)
//	raw.Header = make(http.Header)
//	raw.StatusCode = int(res.Status)
//	for i := 0; i < len(res.Headers); i++ {
//		header := res.Headers[i]
//		raw.Header.Add(header.Name, header.Value)
//	}
//	raw.Body = ioutil.NopCloser(&res.Content)
//	return raw
//}

func (res *HTTPResponseEvent) FromResponse(raw *http.Response) {
	res.rawRes = raw
	res.Status = uint32(raw.StatusCode)
	for key, values := range raw.Header {
		for _, value := range values {
			res.AddHeader(key, value)
		}
	}
}

func (res *HTTPResponseEvent) GetType() uint32 {
	return HTTP_RESPONSE_EVENT_TYPE
}
func (res *HTTPResponseEvent) GetVersion() uint32 {
	return 1
}

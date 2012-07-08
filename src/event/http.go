package event

import (
	"bytes"
	"strconv"
	"strings"
	"util"
)

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
	b, err := DecodeBytesValue(buffer)
	if err != nil {
		return err
	}
	msg.Content.Write(b)
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

func (req *HTTPRequestEvent) GetType() uint32 {
   return HTTP_REQUEST_EVENT_TYPE
}
func (req *HTTPRequestEvent) GetVersion() uint32 {
   return 1
}

type HTTPResponseEvent struct {
	HTTPMessageEvent
	Status uint32
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

func (res *HTTPResponseEvent) GetType() uint32 {
   return HTTP_RESPONSE_EVENT_TYPE
}
func (res *HTTPResponseEvent) GetVersion() uint32 {
   return 1
}

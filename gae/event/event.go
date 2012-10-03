package event

import (
	"bytes"
	"encoding/binary"
	"codec"
	"snappy"
	"se"
	//"fmt"
	//vector "container/vector"
)

const (
	HTTP_REQUEST_EVENT_TYPE                = 1000
	HTTP_RESPONSE_EVENT_TYPE               = 1001
	HTTP_CHUNK_EVENT_TYPE                  = 1002
	HTTP_ERROR_EVENT_TYPE                  = 1003
	HTTP_CONNECTION_EVENT_TYPE             = 1004
	RESERVED_SEGMENT_EVENT_TYPE            = 48100
	COMPRESS_EVENT_TYPE                    = 1500
	ENCRYPT_EVENT_TYPE                     = 1501
	AUTH_REQUEST_EVENT_TYPE                = 2000
	AUTH_RESPONSE_EVENT_TYPE               = 2001
	USER_OPERATION_EVENT_TYPE              = 2010
	GROUP_OPERATION_EVENT_TYPE             = 2011
	USER_LIST_REQUEST_EVENT_TYPE           = 2012
	GROUOP_LIST_REQUEST_EVENT_TYPE         = 2013
	USER_LIST_RESPONSE_EVENT_TYPE          = 2014
	GROUOP_LIST_RESPONSE_EVENT_TYPE        = 2015
	BLACKLIST_OPERATION_EVENT_TYPE         = 2016
	ADMIN_RESPONSE_EVENT_TYPE              = 2020
	SERVER_CONFIG_EVENT_TYPE               = 2050
	REQUEST_SHARED_APPID_EVENT_TYPE        = 2017
	REQUEST_SHARED_APPID_RESULT_EVENT_TYPE = 2018
	SHARE_APPID_EVENT_TYPE                 = 2019
	REQUEST_ALL_SHARED_APPID_EVENT_TYPE                 = 2021
)

const (
	MAGIC_NUMBER uint16 = 0xCAFE
)

func equalIgnoreCase(s1, s2 string) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := 0; i < len(s1); i++ {
		c1 := s1[i]
		if 'A' <= c1 && c1 <= 'Z' {
			c1 += 'a' - 'A'
		}
		c2 := s2[i]
		if 'A' <= c2 && c2 <= 'Z' {
			c2 += 'a' - 'A'
		}
		if c1 != c2 {
			return false
		}
	}
	return true
}

type EventHeaderTags struct {
	magic uint16
	Token string
}

func (tags *EventHeaderTags) Encode(buffer *bytes.Buffer) bool {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, MAGIC_NUMBER)
	buffer.Write(b)
	codec.WriteVarString(buffer, tags.Token)
	return true
}
func (tags *EventHeaderTags) Decode(buffer *bytes.Buffer) bool {
	b := make([]byte, 2)
	realLen, err := buffer.Read(b)
	if err != nil || realLen != 2 {
		return false
	}
	tags.magic = binary.BigEndian.Uint16(b)
	if tags.magic != MAGIC_NUMBER {
		return false
	}
	token, ok := codec.ReadVarString(buffer)
	tags.Token = token
	return ok
}

type Event interface {
	Encode(buffer *bytes.Buffer) bool
	Decode(buffer *bytes.Buffer) bool
	GetType() uint32
	GetVersion() uint32
	GetHash() uint32
	SetHash(hash uint32)
	GetAttachement() interface{}
	SetAttachement(interface{})
}

type Attachement struct {
	attachment interface{}
}

func (att *Attachement) GetAttachement() interface{} {
	return att.attachment
}
func (att *Attachement) SetAttachement(a interface{}) {
	att.attachment = a
}

type HashField struct {
	Hash uint32
}

func (field *HashField) GetHash() uint32 {
	return field.Hash
}
func (field *HashField) SetHash(hash uint32) {
	field.Hash = hash
}

type EventHeader struct {
	Type    uint32
	Version uint32
	Hash    uint32
}

func (header *EventHeader) Encode(buffer *bytes.Buffer) bool {
	codec.WriteUvarint(buffer, uint64(header.Type))
	codec.WriteUvarint(buffer, uint64(header.Version))
	codec.WriteUvarint(buffer, uint64(header.Hash))
	return true
}
func (header *EventHeader) Decode(buffer *bytes.Buffer) bool {
	tmp1, err1 := codec.ReadUvarint(buffer)
	tmp2, err2 := codec.ReadUvarint(buffer)
	tmp3, err3 := codec.ReadUvarint(buffer)
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}
	header.Type, header.Version, header.Hash = uint32(tmp1), uint32(tmp2), uint32(tmp3)
	return true
}

type AdminResponseEvent struct {
	Response  string
	ErrorCause   string
	errno int32
	HashField
	Attachement
}

func (res *AdminResponseEvent) Encode(buffer *bytes.Buffer) bool {
	codec.WriteVarString(buffer, res.Response)
	codec.WriteVarString(buffer, res.ErrorCause)
	codec.WriteUvarint(buffer, uint64(res.errno))
	return true
}
func (res *AdminResponseEvent) Decode(buffer *bytes.Buffer) bool {
	var ok bool
	res.Response, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	res.ErrorCause, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	tmp, err := codec.ReadUvarint(buffer)
	if err != nil  {
		return false
	}
	res.errno = int32(tmp)
	return true
}
func (res *AdminResponseEvent) GetType() uint32 {
	return ADMIN_RESPONSE_EVENT_TYPE
}
func (res *AdminResponseEvent) GetVersion() uint32 {
	return 1
}

type AuthRequestEvent struct {
	Appid  string
	User   string
	Passwd string
	HashField
	Attachement
}

func (req *AuthRequestEvent) Encode(buffer *bytes.Buffer) bool {
	codec.WriteVarString(buffer, req.Appid)
	codec.WriteVarString(buffer, req.User)
	codec.WriteVarString(buffer, req.Passwd)
	return true
}
func (req *AuthRequestEvent) Decode(buffer *bytes.Buffer) bool {
	var ok bool
	req.Appid, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	req.User, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	req.Passwd, ok = codec.ReadVarString(buffer)
	return ok
}
func (req *AuthRequestEvent) GetType() uint32 {
	return AUTH_REQUEST_EVENT_TYPE
}
func (req *AuthRequestEvent) GetVersion() uint32 {
	return 1
}

type AuthResponseEvent struct {
	Appid string
	Token string
	Error string
	HashField
	Attachement
}

func (req *AuthResponseEvent) Encode(buffer *bytes.Buffer) bool {
	codec.WriteVarString(buffer, req.Appid)
	codec.WriteVarString(buffer, req.Token)
	codec.WriteVarString(buffer, req.Error)
	return true
}
func (req *AuthResponseEvent) Decode(buffer *bytes.Buffer) bool {
	var ok bool
	req.Appid, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	req.Token, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	req.Error, ok = codec.ReadVarString(buffer)
	return ok
}
func (req *AuthResponseEvent) GetType() uint32 {
	return AUTH_RESPONSE_EVENT_TYPE
}
func (req *AuthResponseEvent) GetVersion() uint32 {
	return 1
}

type NameValuePair struct{
     Name string
	 Value string
}

type HTTPMessageEvent struct {
	//Headers vector.Vector
	Headers []*NameValuePair
	Content bytes.Buffer
}

func (msg *HTTPMessageEvent) getHeaderEntry(name string) *NameValuePair {
	slen := len(msg.Headers)
	for i := 0; i < slen; i++ {
		header := msg.Headers[i]
		if header.Name == name {
			return header
		}
	}
	return nil
}

func (msg *HTTPMessageEvent) SetHeader(name, value string) {
	he := msg.getHeaderEntry(name)
	if nil != he {
		he.Value = value
	} else {
	    pair := new(NameValuePair)
		pair.Name = name
		pair.Value = value
		//msg.Headers.Push(pair)
		msg.Headers = append(msg.Headers, pair)
	}
}

func (msg *HTTPMessageEvent) AddHeader(name, value string) {
	pair := new(NameValuePair)
	pair.Name = name
	pair.Value = value
	msg.Headers = append(msg.Headers, pair)
}

func (msg *HTTPMessageEvent) GetHeader(name string) string {
	he := msg.getHeaderEntry(name)
	if nil != he {
		return he.Name
	}
	return ""
}

func (msg *HTTPMessageEvent) DoEncode(buffer *bytes.Buffer) bool {
	var slen int = len(msg.Headers)
	codec.WriteUvarint(buffer, uint64(slen))
	for i := 0; i < slen; i++ {
		header:= msg.Headers[i]
		codec.WriteVarString(buffer, header.Name)
	    codec.WriteVarString(buffer, header.Value)
	}
	b := msg.Content.Bytes()
	codec.WriteVarBytes(buffer, b)
	return true
}

func (msg *HTTPMessageEvent) DoDecode(buffer *bytes.Buffer) bool {
	length, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	for i := 0; i < int(length); i++ {
		headerName, ok := codec.ReadVarString(buffer)
		if !ok {
			return false
		}
		headerValue, ok := codec.ReadVarString(buffer)
		if !ok {
			return false
		}
		pair := new(NameValuePair)
		pair.Name = headerName
		pair.Value = headerValue
		msg.Headers = append(msg.Headers, pair)
	}
	b, ok := codec.ReadVarBytes(buffer)
	if !ok {
		return false
	}
	msg.Content.Write(b)
	return true
}

type HTTPRequestEvent struct {
	HTTPMessageEvent
	Method string
	Url    string
	HashField
	Attachement
}

func (req *HTTPRequestEvent) Encode(buffer *bytes.Buffer) bool {
	codec.WriteVarString(buffer, req.Url)
	codec.WriteVarString(buffer, req.Method)
	req.DoEncode(buffer)
	return true
}
func (req *HTTPRequestEvent) Decode(buffer *bytes.Buffer) bool {
	var ok bool
	req.Url, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	req.Method, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	ok = req.DoDecode(buffer)
	if !ok {
		return false
	}
	return true
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
	HashField
	Attachement
}

func (res *HTTPResponseEvent) Encode(buffer *bytes.Buffer) bool {
	codec.WriteUvarint(buffer, uint64(res.Status))
	res.DoEncode(buffer)
	return true
}
func (res *HTTPResponseEvent) Decode(buffer *bytes.Buffer) bool {
	tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	res.Status = uint32(tmp)
	ok := res.DoDecode(buffer)
	if !ok {
		return false
	}
	return true
}

func (req *HTTPResponseEvent) GetType() uint32 {
	return HTTP_RESPONSE_EVENT_TYPE
}
func (req *HTTPResponseEvent) GetVersion() uint32 {
	return 1
}

type SegmentEvent struct {
	Sequence uint32
	Total    uint32
	Content  bytes.Buffer
	HashField
	Attachement
}

func (seg *SegmentEvent) Encode(buffer *bytes.Buffer) bool {
	codec.WriteUvarint(buffer, uint64(seg.Sequence))
	codec.WriteUvarint(buffer, uint64(seg.Total))
	codec.WriteVarBytes(buffer, seg.Content.Bytes())
	return true
}
func (seg *SegmentEvent) Decode(buffer *bytes.Buffer) bool {
	tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	seg.Sequence = uint32(tmp)
	tmp, err = codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	seg.Total = uint32(tmp)
	length, err := codec.ReadUvarint(buffer)
	buf := make([]byte, length)
	realLen, err := buffer.Read(buf)
	if err != nil || uint64(realLen) < length {
		return false
	}
	seg.Content.Write(buf)
	return true
}

func (seg *SegmentEvent) GetType() uint32 {
	return RESERVED_SEGMENT_EVENT_TYPE
}
func (seg *SegmentEvent) GetVersion() uint32 {
	return 1
}

const (
	C_NONE    uint32 = 0
	C_SNAPPY  uint32 = 1
	C_LZF     uint32 = 2
	C_FASTLZ  uint32 = 3
	C_QUICKLZ uint32 = 4
)

type CompressEvent struct {
	CompressType uint32
	Ev           Event
	HashField
	Attachement
}

func (ev *CompressEvent) Encode(buffer *bytes.Buffer) bool {
	if ev.CompressType != C_NONE && ev.CompressType != C_SNAPPY {
		ev.CompressType = C_NONE
	}
	codec.WriteUvarint(buffer, uint64(ev.CompressType))
	//ev.ev.Encode(buffer);
	var buf bytes.Buffer
	EncodeEvent(&buf, ev.Ev)
	switch ev.CompressType {
	case C_NONE:
		{
			buffer.Write(buf.Bytes())
		}
	case C_SNAPPY:
		{
			evbuf := make([]byte, 0)
			newbuf := snappy.Encode(evbuf, buf.Bytes())
			buffer.Write(newbuf)
		}
	}
	return true
}
func (ev *CompressEvent) Decode(buffer *bytes.Buffer) bool {
	tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	ev.CompressType = uint32(tmp)
	var success bool
	switch ev.CompressType {
	case C_NONE:
		{
			success, ev.Ev, _ = ParseEvent(buffer)
			return success
		}
	case C_SNAPPY:
		{
			b := make([]byte, 0, 0)
			newbuf, ok, _ := snappy.Decode(b, buffer.Bytes())
			if !ok {
				return false
			}
			success, ev.Ev, _ = ParseEvent(bytes.NewBuffer(newbuf))
			return success
		}
		default: return false
	}
	return true
}

func (ev *CompressEvent) GetType() uint32 {
	return COMPRESS_EVENT_TYPE
}
func (ev *CompressEvent) GetVersion() uint32 {
	return 1
}

const (
	E_NONE uint32 = 0
	E_SE1  uint32 = 1
)

type EncryptEvent struct {
	EncryptType uint32
	Ev          Event
	HashField
	Attachement
}

func (ev *EncryptEvent) Encode(buffer *bytes.Buffer) bool {
	codec.WriteUvarint(buffer, uint64(ev.EncryptType))
	//ev.ev.Encode(buffer);
	buf := new(bytes.Buffer)
	EncodeEvent(buf, ev.Ev)
	switch ev.EncryptType {
	case E_NONE:
		{
			buffer.Write(buf.Bytes())
		}
	case E_SE1:
		{
			newbuf := se.Encrypt(buf)
			buffer.Write(newbuf.Bytes())
		}
	}
	return true
}
func (ev *EncryptEvent) Decode(buffer *bytes.Buffer) bool {
	tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	ev.EncryptType = uint32(tmp)
	var success bool
	switch ev.EncryptType {
	case E_NONE:
		{
			success, ev.Ev, _ = ParseEvent(buffer)
			return success
		}
	case E_SE1:
		{
			newbuf := se.Decrypt(buffer)
			success, ev.Ev, _ = ParseEvent(newbuf)
			return success
		}
	}
	return true
}

func (ev *EncryptEvent) GetType() uint32 {
	return ENCRYPT_EVENT_TYPE
}
func (ev *EncryptEvent) GetVersion() uint32 {
	return 1
}

const(
   OPERATION_ADD = 0
   OPERATION_DELETE = 1
   OPERATION_MODIFY = 2
)

type UserOperationEvent struct {
	User      User
	Operation uint32
	HashField
	Attachement
}

func (ev *UserOperationEvent) Encode(buffer *bytes.Buffer) bool {
	ev.User.Encode(buffer)
	codec.WriteUvarint(buffer, uint64(ev.Operation))
	return true
}
func (ev *UserOperationEvent) Decode(buffer *bytes.Buffer) bool {
	if !ev.User.Decode(buffer) {
		return false
	}
	tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	ev.Operation = uint32(tmp)
	return true
}

func (ev *UserOperationEvent) GetType() uint32 {
	return USER_OPERATION_EVENT_TYPE
}
func (ev *UserOperationEvent) GetVersion() uint32 {
	return 1
}

type GroupOperationEvent struct {
	Group     Group
	Operation uint32
	HashField
	Attachement
}

func (ev *GroupOperationEvent) Encode(buffer *bytes.Buffer) bool {
	ev.Group.Encode(buffer)
	codec.WriteUvarint(buffer, uint64(ev.Operation))
	return true
}
func (ev *GroupOperationEvent) Decode(buffer *bytes.Buffer) bool {
	if !ev.Group.Decode(buffer) {
		return false
	}
	tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	ev.Operation = uint32(tmp)
	return true
}

func (ev *GroupOperationEvent) GetType() uint32 {
	return GROUP_OPERATION_EVENT_TYPE
}
func (ev *GroupOperationEvent) GetVersion() uint32 {
	return 1
}

const (
	GET_CONFIG_REQ uint32 = 1
	GET_CONFIG_RES uint32 = 2
	SET_CONFIG_REQ uint32 = 3
	SET_CONFIG_RES uint32 = 4
)

type ServerConfigEvent struct {
	Cfg       *GAEServerConfig
	Operation uint32
	HashField
	Attachement
}

func (ev *ServerConfigEvent) Encode(buffer *bytes.Buffer) bool {
	codec.WriteUvarint(buffer, uint64(ev.Operation))
	ev.Cfg.Encode(buffer)
	return true
}
func (ev *ServerConfigEvent) Decode(buffer *bytes.Buffer) bool {
    tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	ev.Cfg = new(GAEServerConfig)
	if !ev.Cfg.Decode(buffer) {
		return false
	}
	
	ev.Operation = uint32(tmp)
	return true
}

func (ev *ServerConfigEvent) GetType() uint32 {
	return SERVER_CONFIG_EVENT_TYPE
}
func (ev *ServerConfigEvent) GetVersion() uint32 {
	return 1
}

type ListGroupRequestEvent struct {
	HashField
	Attachement
}

func (ev *ListGroupRequestEvent) Encode(buffer *bytes.Buffer) bool {
	return true
}
func (ev *ListGroupRequestEvent) Decode(buffer *bytes.Buffer) bool {
	return true
}

func (ev *ListGroupRequestEvent) GetType() uint32 {
	return GROUOP_LIST_REQUEST_EVENT_TYPE
}
func (ev *ListGroupRequestEvent) GetVersion() uint32 {
	return 1
}

type ListGroupResponseEvent struct {
    Groups []*Group
	HashField
	Attachement
}

func (ev *ListGroupResponseEvent) Encode(buffer *bytes.Buffer) bool {
    codec.WriteUvarint(buffer, uint64(len(ev.Groups)))
	for _,group:= range ev.Groups{
	   group.Encode(buffer)
	}
	return true
}
func (ev *ListGroupResponseEvent) Decode(buffer *bytes.Buffer) bool {
    tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	ev.Groups = make([]*Group, int(tmp))
	for i:=0 ; i< int(tmp);i++{
	   ev.Groups[i] = new(Group)
	   if !ev.Groups[i].Decode(buffer){
	      return false
	   }
	}
	return true
}

func (ev *ListGroupResponseEvent) GetType() uint32 {
	return GROUOP_LIST_RESPONSE_EVENT_TYPE
}
func (ev *ListGroupResponseEvent) GetVersion() uint32 {
	return 1
}

type ListUserRequestEvent struct {
	HashField
	Attachement
}

func (ev *ListUserRequestEvent) Encode(buffer *bytes.Buffer) bool {
	return true
}
func (ev *ListUserRequestEvent) Decode(buffer *bytes.Buffer) bool {
	return true
}

func (ev *ListUserRequestEvent) GetType() uint32 {
	return USER_LIST_REQUEST_EVENT_TYPE
}
func (ev *ListUserRequestEvent) GetVersion() uint32 {
	return 1
}

type ListUserResponseEvent struct {
    Users []*User
	HashField
	Attachement
}

func (ev *ListUserResponseEvent) Encode(buffer *bytes.Buffer) bool {
    codec.WriteUvarint(buffer, uint64(len(ev.Users)))
	for _,user:= range ev.Users{
	   user.Encode(buffer)
	}
	return true
}
func (ev *ListUserResponseEvent) Decode(buffer *bytes.Buffer) bool {
    tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	ev.Users = make([]*User, int(tmp))
	for i:=0 ; i< int(tmp);i++{
	   ev.Users[i] = new(User)
	   if !ev.Users[i].Decode(buffer){
	      return false
	   }
	}
	return true
}

func (ev *ListUserResponseEvent) GetType() uint32 {
	return USER_LIST_RESPONSE_EVENT_TYPE
}
func (ev *ListUserResponseEvent) GetVersion() uint32 {
	return 1
}

const (
	BLACKLIST_ADD    uint32 = 0
	BLACKLIST_DELETE uint32 = 1
	BLACKLIST_MODIFY uint32 = 2
)

type BlackListOperationEvent struct {
	User      string
	Group     string
	Host      string
	Operation uint32
	HashField
	Attachement
}

func (ev *BlackListOperationEvent) Encode(buffer *bytes.Buffer) bool {
	codec.WriteVarString(buffer, ev.User)
	codec.WriteVarString(buffer, ev.Group)
	codec.WriteVarString(buffer, ev.Host)
	codec.WriteUvarint(buffer, uint64(ev.Operation))
	return true
}
func (ev *BlackListOperationEvent) Decode(buffer *bytes.Buffer) bool {
	var ok bool
	ev.User, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	ev.Group, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	ev.Host, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	ev.Operation = uint32(tmp)
	return true
}

func (ev *BlackListOperationEvent) GetType() uint32 {
	return BLACKLIST_OPERATION_EVENT_TYPE
}
func (ev *BlackListOperationEvent) GetVersion() uint32 {
	return 1
}

const (
	APPID_SHARE uint32 = 0
	APPID_UNSHARE  uint32 = 1
)

type ShareAppIDEvent struct{
    Operation uint32
	AppId string
	Email string
	HashField
	Attachement
}

func (ev *ShareAppIDEvent) Encode(buffer *bytes.Buffer) bool {
	codec.WriteUvarint(buffer, uint64(ev.Operation))
	codec.WriteVarString(buffer, ev.AppId)
	codec.WriteVarString(buffer, ev.Email)
	return true
}
func (ev *ShareAppIDEvent) Decode(buffer *bytes.Buffer) bool {
	tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	ev.Operation = uint32(tmp)
	var ok bool
	ev.AppId, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	ev.Email, ok = codec.ReadVarString(buffer)
	if !ok {
		return false
	}
	return true
}

func (ev *ShareAppIDEvent) GetType() uint32 {
	return SHARE_APPID_EVENT_TYPE
}
func (ev *ShareAppIDEvent) GetVersion() uint32 {
	return 1
}

type RequestAppIDEvent struct{
	HashField
	Attachement
}

func (ev *RequestAppIDEvent) Encode(buffer *bytes.Buffer) bool {
	return true
}
func (ev *RequestAppIDEvent) Decode(buffer *bytes.Buffer) bool {
	return true
}

func (ev *RequestAppIDEvent) GetType() uint32 {
	return REQUEST_SHARED_APPID_EVENT_TYPE
}
func (ev *RequestAppIDEvent) GetVersion() uint32 {
	return 1
}

type RequestAllAppIDEvent struct{
	HashField
	Attachement
}

func (ev *RequestAllAppIDEvent) Encode(buffer *bytes.Buffer) bool {
	return true
}
func (ev *RequestAllAppIDEvent) Decode(buffer *bytes.Buffer) bool {
	return true
}

func (ev *RequestAllAppIDEvent) GetType() uint32 {
	return REQUEST_ALL_SHARED_APPID_EVENT_TYPE
}
func (ev *RequestAllAppIDEvent) GetVersion() uint32 {
	return 1
}

type RequestAppIDResponseEvent struct{
    AppIDs []string
	HashField
	Attachement
}

func (ev *RequestAppIDResponseEvent) Encode(buffer *bytes.Buffer) bool {
    if nil == ev.AppIDs{
	   codec.WriteUvarint(buffer, 0)
	   return true;
    }
    codec.WriteUvarint(buffer, uint64(len(ev.AppIDs)))
	for _,appid:= range ev.AppIDs{
	   codec.WriteVarString(buffer, appid)
	}
	return true
}
func (ev *RequestAppIDResponseEvent) Decode(buffer *bytes.Buffer) bool {
	tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	ev.AppIDs = make([]string, int(tmp))
	var ok bool
	for i:=0 ; i< int(tmp);i++{
	   ev.AppIDs[i], ok = codec.ReadVarString(buffer)
	   if !ok {
		  return false
	   }
	}
	return true
}

func (ev *RequestAppIDResponseEvent) GetType() uint32 {
	return REQUEST_SHARED_APPID_RESULT_EVENT_TYPE
}
func (ev *RequestAppIDResponseEvent) GetVersion() uint32 {
	return 1
}

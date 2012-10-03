package event

import (
	"bytes"
	"strconv"
)

type EventHandler interface {
	OnEvent(header *EventHeader, event Event)
}

type EventRegisterValue struct {
	creator func(Type uint32, Version uint32) Event
	handler EventHandler
}

var RegistedEventTable map[uint64]EventRegisterValue = make(map[uint64]EventRegisterValue)

func getEventHandler(ev Event) EventHandler {
	var typeVer uint64
	typeVer = uint64(ev.GetType())<<32 + uint64(ev.GetVersion())
	handler, ok := RegistedEventTable[typeVer]
	if ok {
		return handler.handler
	}
	return nil
}

func ParseEvent(buffer *bytes.Buffer) (bool, Event, string) {
	var header EventHeader
	if !header.Decode(buffer) {
		return false, nil, "failed to dcode event header"
	}
	var typeVer uint64
	typeVer = uint64(header.Type)<<32 + uint64(header.Version)
	value, ok := RegistedEventTable[typeVer]
	if !ok {
		return false, nil, "failed to find register event:" + strconv.FormatInt(int64(header.Type), 10)
	}
	var event Event
	event = value.creator(header.Type, header.Version)
	event.SetHash(header.Hash)
	return event.Decode(buffer), event, ""
}

func ParseEventWithTags(buf []byte) (bool, Event, *EventHeaderTags, string) {
	var buffer = bytes.NewBuffer(buf)
	tags := new(EventHeaderTags)
	ok := tags.Decode(buffer)
	if !ok {
		return false, nil, nil, "Failed to decode header tags"
	}
	success, ev, cause := ParseEvent(buffer)
	return success, ev, tags, cause
}

func CreateEvent(Type uint32, Version uint32) Event {
	switch Type {
	case HTTP_REQUEST_EVENT_TYPE:
		return new(HTTPRequestEvent)
	case HTTP_RESPONSE_EVENT_TYPE:
		return new(HTTPResponseEvent)
	case RESERVED_SEGMENT_EVENT_TYPE:
		return new(SegmentEvent)
	case COMPRESS_EVENT_TYPE:
		return new(CompressEvent)
	case ENCRYPT_EVENT_TYPE:
		return new(EncryptEvent)
	case AUTH_REQUEST_EVENT_TYPE:
		return new(AuthRequestEvent)
	case USER_OPERATION_EVENT_TYPE:
		return new(UserOperationEvent)
	case GROUP_OPERATION_EVENT_TYPE:
		return new(GroupOperationEvent)
	case USER_LIST_REQUEST_EVENT_TYPE:
		return new(ListUserRequestEvent)
	case GROUOP_LIST_REQUEST_EVENT_TYPE:
		return new(ListGroupRequestEvent)
	case BLACKLIST_OPERATION_EVENT_TYPE:
		return new(BlackListOperationEvent)
	case REQUEST_SHARED_APPID_EVENT_TYPE:
		return new(RequestAppIDEvent)
	case REQUEST_ALL_SHARED_APPID_EVENT_TYPE:
		return new(RequestAllAppIDEvent)
	case SHARE_APPID_EVENT_TYPE:
		return new(ShareAppIDEvent)
	case SERVER_CONFIG_EVENT_TYPE:
		return new(ServerConfigEvent)
	}
	return nil
}

func RegisterEventHandler(ev Event, handler EventHandler) (ok bool, err string) {
	if nil == ev {
		return false, "Nil event!"
	}
	tmp1 := ev.GetType()
	tmp2 := ev.GetVersion()
	var key uint64 = uint64(tmp1)<<32 + uint64(tmp2)
	tmp, exist := RegistedEventTable[key]
	if exist {
		return false, "Duplicate event type"
	}
	tmp.creator = CreateEvent
	tmp.handler = handler
	RegistedEventTable[key] = tmp
	return true, ""
}

func DiaptchEvent(ev Event) {
	handler := getEventHandler(ev)
	if nil != handler {
		var header EventHeader
		header.Type = ev.GetType()
		header.Version = ev.GetVersion()
		header.Hash = ev.GetHash()
		handler.OnEvent(&header, ev)
	}
}

func EncodeEvent(buffer *bytes.Buffer, ev Event) bool {
	header := EventHeader{ev.GetType(), ev.GetVersion(), ev.GetHash()}
	header.Encode(buffer)
	return ev.Encode(buffer)
}

func DecodeEvent(buffer *bytes.Buffer) (bool, Event, string) {
	return ParseEvent(buffer)
}

func EncodeEventWithTags(buffer *bytes.Buffer, ev Event, tags *EventHeaderTags) bool {
	tags.Encode(buffer)
	return EncodeEvent(buffer, ev)
}

func InitEvents(handler EventHandler) {
	RegisterEventHandler(new(HTTPRequestEvent), handler)
	RegisterEventHandler(new(SegmentEvent), handler)
	RegisterEventHandler(new(EncryptEvent), handler)
	RegisterEventHandler(new(CompressEvent), handler)
	RegisterEventHandler(new(AuthRequestEvent), handler)
	RegisterEventHandler(new(UserOperationEvent), handler)
	RegisterEventHandler(new(GroupOperationEvent), handler)
	RegisterEventHandler(new(ListGroupRequestEvent), handler)
	RegisterEventHandler(new(ListUserRequestEvent), handler)
	RegisterEventHandler(new(BlackListOperationEvent), handler)
	RegisterEventHandler(new(ServerConfigEvent), handler)
	RegisterEventHandler(new(RequestAppIDEvent), handler)
	RegisterEventHandler(new(ShareAppIDEvent), handler)
	RegisterEventHandler(new(RequestAllAppIDEvent), handler)
}

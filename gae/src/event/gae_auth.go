package event

import (
	"bytes"
)

type SegmentEvent struct {
	Sequence uint32
	Total    uint32
	Content  []byte
	EventHeader
}

func (seg *SegmentEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt32Value(buffer, seg.Sequence)
	EncodeUInt32Value(buffer, seg.Total)
	EncodeBytesValue(buffer, seg.Content)
}
func (seg *SegmentEvent) Decode(buffer *bytes.Buffer) (err error) {
	if seg.Sequence, err = DecodeUInt32Value(buffer); nil == err {
		if seg.Total, err = DecodeUInt32Value(buffer); nil == err {
			seg.Content, err = DecodeBytesValue(buffer)
		}
	}
	return
}

func (seg *SegmentEvent) GetType() uint32 {
	return RESERVED_SEGMENT_EVENT_TYPE
}
func (seg *SegmentEvent) GetVersion() uint32 {
	return 1
}

const (
	OPERATION_ADD    = 0
	OPERATION_DELETE = 1
	OPERATION_MODIFY = 2
)

type UserOperationEvent struct {
	User      User
	Operation uint32
	EventHeader
}

func (ev *UserOperationEvent) Encode(buffer *bytes.Buffer) {
	ev.User.Encode(buffer)
	EncodeUInt32Value(buffer, ev.Operation)
}
func (ev *UserOperationEvent) Decode(buffer *bytes.Buffer) (err error) {
	if err := ev.User.Decode(buffer); nil != err {
		return err
	}
	ev.Operation, err = DecodeUInt32Value(buffer)
	return
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
	EventHeader
}

func (ev *GroupOperationEvent) Encode(buffer *bytes.Buffer) {
	ev.Group.Encode(buffer)
	EncodeUInt32Value(buffer, ev.Operation)
}
func (ev *GroupOperationEvent) Decode(buffer *bytes.Buffer) (err error) {
	if err := ev.Group.Decode(buffer); nil != err {
		return err
	}
	ev.Operation, err = DecodeUInt32Value(buffer)
	return
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
	EventHeader
}

func (ev *ServerConfigEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt32Value(buffer, ev.Operation)
	ev.Cfg.Encode(buffer)
}
func (ev *ServerConfigEvent) Decode(buffer *bytes.Buffer) (err error) {
	ev.Operation, err = DecodeUInt32Value(buffer)
	if nil != err {
		return
	}
	ev.Cfg = new(GAEServerConfig)
	err = ev.Cfg.Decode(buffer)
	return
}

func (ev *ServerConfigEvent) GetType() uint32 {
	return SERVER_CONFIG_EVENT_TYPE
}
func (ev *ServerConfigEvent) GetVersion() uint32 {
	return 1
}

type ListGroupRequestEvent struct {
	EventHeader
}

func (ev *ListGroupRequestEvent) Encode(buffer *bytes.Buffer) {
}
func (ev *ListGroupRequestEvent) Decode(buffer *bytes.Buffer) error {
	return nil
}

func (ev *ListGroupRequestEvent) GetType() uint32 {
	return GROUOP_LIST_REQUEST_EVENT_TYPE
}
func (ev *ListGroupRequestEvent) GetVersion() uint32 {
	return 1
}

type ListGroupResponseEvent struct {
	Groups []*Group
	EventHeader
}

func (ev *ListGroupResponseEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt32Value(buffer, uint32(len(ev.Groups)))
	for _, group := range ev.Groups {
		group.Encode(buffer)
	}
}
func (ev *ListGroupResponseEvent) Decode(buffer *bytes.Buffer) (err error) {
	tmp, err := DecodeUInt32Value(buffer)
	if nil != err {
		return err
	}
	ev.Groups = make([]*Group, int(tmp))
	for i := 0; i < int(tmp); i++ {
		ev.Groups[i] = new(Group)
		if err = ev.Groups[i].Decode(buffer); nil != err {
			return err
		}
	}
	return nil
}

func (ev *ListGroupResponseEvent) GetType() uint32 {
	return GROUOP_LIST_RESPONSE_EVENT_TYPE
}
func (ev *ListGroupResponseEvent) GetVersion() uint32 {
	return 1
}

type ListUserRequestEvent struct {
	EventHeader
}

func (ev *ListUserRequestEvent) Encode(buffer *bytes.Buffer) {
}
func (ev *ListUserRequestEvent) Decode(buffer *bytes.Buffer) error {
	return nil
}

func (ev *ListUserRequestEvent) GetType() uint32 {
	return USER_LIST_REQUEST_EVENT_TYPE
}
func (ev *ListUserRequestEvent) GetVersion() uint32 {
	return 1
}

type ListUserResponseEvent struct {
	Users []*User
	EventHeader
}

func (ev *ListUserResponseEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt32Value(buffer, uint32(len(ev.Users)))
	for _, user := range ev.Users {
		user.Encode(buffer)
	}
}
func (ev *ListUserResponseEvent) Decode(buffer *bytes.Buffer) (err error) {
	tmp, err := DecodeUInt32Value(buffer)
	if nil != err {
		return err
	}
	ev.Users = make([]*User, int(tmp))
	for i := 0; i < int(tmp); i++ {
		ev.Users[i] = new(User)
		if err = ev.Users[i].Decode(buffer); nil != err {
			return err
		}
	}
	return nil
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
	EventHeader
}

func (ev *BlackListOperationEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, ev.User)
	EncodeStringValue(buffer, ev.Group)
	EncodeStringValue(buffer, ev.Host)
	EncodeUInt32Value(buffer, ev.Operation)
}
func (ev *BlackListOperationEvent) Decode(buffer *bytes.Buffer) (err error) {
	if ev.User, err = DecodeStringValue(buffer); nil == err {
		if ev.Group, err = DecodeStringValue(buffer); nil == err {
			if ev.Host, err = DecodeStringValue(buffer); nil == err {
				ev.Operation, err = DecodeUInt32Value(buffer)
			}
		}
	}
	return
}

func (ev *BlackListOperationEvent) GetType() uint32 {
	return BLACKLIST_OPERATION_EVENT_TYPE
}
func (ev *BlackListOperationEvent) GetVersion() uint32 {
	return 1
}

func InitGAEAuthEvents() {
	RegistEvent(&BlackListOperationEvent{})
	RegistEvent(&SegmentEvent{})
	RegistEvent(&UserOperationEvent{})
	RegistEvent(&GroupOperationEvent{})
	RegistEvent(&ServerConfigEvent{})
	RegistEvent(&ListGroupRequestEvent{})
	RegistEvent(&ListUserRequestEvent{})
	RegistEvent(&ShareAppIDEvent{})
}

package event

import (
	"bytes"
)

type TCPChunkEvent struct {
	Sequence uint32
	Content  []byte
	EventHeader
}

func (req *TCPChunkEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt32Value(buffer, req.Sequence)
	EncodeBytesValue(buffer, req.Content)
}
func (req *TCPChunkEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.Sequence, err = DecodeUInt32Value(buffer)
	if err != nil {
		return
	}
	req.Content, err = DecodeBytesValue(buffer)
	if err != nil {
		return
	}
	return nil
}

func (req *TCPChunkEvent) GetType() uint32 {
	return EVENT_TCP_CHUNK_TYPE
}
func (req *TCPChunkEvent) GetVersion() uint32 {
	return 1
}

type SocketConnectionEvent struct {
	Status uint32
	Addr  string
	EventHeader
}

func (req *SocketConnectionEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt32Value(buffer, req.Status)
	EncodeStringValue(buffer, req.Addr)
}
func (req *SocketConnectionEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.Status, err = DecodeUInt32Value(buffer)
	if err != nil {
		return
	}
	req.Addr, err = DecodeStringValue(buffer)
	if err != nil {
		return
	}
	return nil
}

func (req *SocketConnectionEvent) GetType() uint32 {
	return EVENT_TCP_CONNECTION_TYPE
}
func (req *SocketConnectionEvent) GetVersion() uint32 {
	return 1
}

type UserLoginEvent struct {
	User  string
	EventHeader
}

func (req *UserLoginEvent) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, req.User)
}
func (req *UserLoginEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.User, err = DecodeStringValue(buffer)
	if err != nil {
		return
	}
	return nil
}

func (req *UserLoginEvent) GetType() uint32 {
	return EVENT_USER_LOGIN_TYPE
}
func (req *UserLoginEvent) GetVersion() uint32 {
	return 1
}

//type EventRestRequest struct {
//	RestSessions []uint32
//	EventHeader
//}
//
//func (req *EventRestRequest) Encode(buffer *bytes.Buffer) {
//	EncodeUInt64Value(buffer, uint64(len(req.RestSessions)))
//	for i := 0; i < len(req.RestSessions); i++ {
//		EncodeUInt64Value(buffer, uint64(req.RestSessions[i]))
//	}
//}
//func (req *EventRestRequest) Decode(buffer *bytes.Buffer) (err error) {
//	length, err := DecodeUInt32Value(buffer)
//	if err != nil {
//		return
//	}
//	req.RestSessions = make([]uint32, length)
//	for i := 0; i < int(length); i++ {
//		req.RestSessions[i], err = DecodeUInt32Value(buffer)
//		if err != nil {
//			return
//		}
//	}
//	return nil
//}

//func (req *EventRestRequest) GetType() uint32 {
//	return EVENT_REST_REQEUST_TYPE
//}
//func (req *EventRestRequest) GetVersion() uint32 {
//	return 1
//}
//
//type EventRestNotify struct {
//	Rest         uint32
//	RestSessions []uint32
//	EventHeader
//}
//
//func (req *EventRestNotify) Encode(buffer *bytes.Buffer) {
//	EncodeInt64Value(buffer, int64(req.Rest))
//	EncodeInt64Value(buffer, int64(len(req.RestSessions)))
//	for _, v := range req.RestSessions {
//		EncodeInt64Value(buffer, int64(v))
//	}
//}
//func (req *EventRestNotify) Decode(buffer *bytes.Buffer) (err error) {
//	req.Rest, err = DecodeUInt32Value(buffer)
//	if err != nil {
//		return
//	}
//	length, err := DecodeUInt32Value(buffer)
//	if err != nil {
//		return
//	}
//	req.RestSessions = make([]uint32, length)
//	for i := 0; i < int(length); i++ {
//		req.RestSessions[i], err = DecodeUInt32Value(buffer)
//		if err != nil {
//			return
//		}
//	}
//	return nil
//}
//
//func (req *EventRestNotify) GetType() uint32 {
//	return EVENT_REST_NOTIFY_TYPE
//}
//func (req *EventRestNotify) GetVersion() uint32 {
//	return 1
//}
//
//type SequentialChunkEvent struct {
//	Sequence uint32
//	Content  []byte
//	EventHeader
//}
//
//func (req *SequentialChunkEvent) Encode(buffer *bytes.Buffer) {
//	EncodeUInt32Value(buffer, req.Sequence)
//	EncodeBytesValue(buffer, req.Content)
//}
//func (req *SequentialChunkEvent) Decode(buffer *bytes.Buffer) (err error) {
//	req.Sequence, err = DecodeUInt32Value(buffer)
//	if err != nil {
//		return
//	}
//	req.Content, err = DecodeBytesValue(buffer)
//	if err != nil {
//		return
//	}
//	return nil
//}
//
//func (req *SequentialChunkEvent) GetType() uint32 {
//	return EVENT_SEQUNCEIAL_CHUNK_TYPE
//}
//func (req *SequentialChunkEvent) GetVersion() uint32 {
//	return 1
//}
//
//type TransactionCompleteEvent struct {
//	EventHeader
//}
//
//func (req *TransactionCompleteEvent) Encode(buffer *bytes.Buffer) {
//}
//func (req *TransactionCompleteEvent) Decode(buffer *bytes.Buffer) (err error) {
//	return nil
//}
//
//func (req *TransactionCompleteEvent) GetType() uint32 {
//	return EVENT_TRANSACTION_COMPLETE_TYPE
//}
//func (req *TransactionCompleteEvent) GetVersion() uint32 {
//	return 1
//}

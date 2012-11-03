package event

import (
	"errors"
	"reflect"
	"strconv"
)

type EventTypeKey struct {
	Type    uint32
	Version uint32
}

var registed_events map[EventTypeKey]reflect.Type = make(map[EventTypeKey]reflect.Type)
var registed_types map[reflect.Type]EventTypeKey = make(map[reflect.Type]EventTypeKey)

func RegistObject(event_type, event_version uint32, ev interface{}) {
	rt := reflect.TypeOf(ev)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	tk := EventTypeKey{event_type, event_version}
	registed_events[tk] = rt
	registed_types[rt] = tk
}

func RegistEvent(ev Event) {
	RegistObject(ev.GetType(), ev.GetVersion(), ev)
}

func GetRegistTypeVersion(obj interface{}) (exist bool, tk EventTypeKey) {
	rt := reflect.TypeOf(obj)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	if t, ok := registed_types[rt]; !ok {
		exist = false
	} else {
		exist = true
		tk = t
	}
	return
}

func NewObjectInstance(event_type, event_version uint32) (err error, ev interface{}) {
	key := EventTypeKey{event_type, event_version}
	if t, ok := registed_events[key]; !ok {
		err = errors.New("No registe event found for [" + strconv.Itoa(int(event_type)) + ":" + strconv.Itoa(int(event_version)) + "]")
	} else {
		v := reflect.New(t)
		ev = v.Interface()
	}
	return
}

func NewEventInstance(event_type, event_version uint32) (err error, ev Event) {
	key := EventTypeKey{event_type, event_version}
	if t, ok := registed_events[key]; !ok {
		err = errors.New("No registe event found for [" + strconv.Itoa(int(event_type)) + ":" + strconv.Itoa(int(event_version)) + "]")
	} else {
		v := reflect.New(t)
		ev = v.Interface().(Event)
	}
	return
}

func Init() {
	RegistEvent(&AuthRequestEvent{})
	RegistEvent(&AuthResponseEvent{})
	RegistEvent(&EncryptEvent{})
	RegistEvent(&CompressEvent{})
	RegistEvent(&EncryptEventV2{})
	RegistEvent(&CompressEventV2{})
	RegistEvent(&HTTPResponseEvent{})
	RegistEvent(&HTTPRequestEvent{})
	RegistEvent(&RequestAppIDResponseEvent{})
	RegistEvent(&HTTPConnectionEvent{})
	RegistEvent(&HTTPErrorEvent{})
	RegistEvent(&TCPChunkEvent{})
	RegistEvent(&SocketConnectionEvent{})
	RegistEvent(&UserLoginEvent{})
	RegistEvent(&RSocketAcceptedEvent{})
	RegistEvent(&AdminResponseEvent{})
	RegistEvent(&RequestAppIDEvent{})
}

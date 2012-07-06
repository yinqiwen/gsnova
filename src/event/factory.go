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

func RegistEvent(event_type, event_version uint32, ev interface{}) {
	rt := reflect.TypeOf(ev)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	registed_events[EventTypeKey{event_type, event_version}] = rt
}

func NewEventInstance(event_type, event_version uint32) (err error, ev interface{}) {
	key := EventTypeKey{event_type, event_version}
	if t, ok := registed_events[key]; !ok {
		err = errors.New("No registe event found for [" + strconv.Itoa(int(event_type)) + ":" + strconv.Itoa(int(event_version)) + "]")
	} else {
		v := reflect.New(t)
		ev = v.Interface()
	}
	return
}

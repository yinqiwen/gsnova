package event

import (
	"errors"
	"reflect"
	"strconv"
)

var registedEvents map[uint16]reflect.Type = make(map[uint16]reflect.Type)
var registedTypes map[reflect.Type]uint16 = make(map[reflect.Type]uint16)

func RegistObject(eventType uint16, ev interface{}) {
	rt := reflect.TypeOf(ev)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	registedEvents[eventType] = rt
	registedTypes[rt] = eventType
}

// func RegistEvent(ev Event) {
// 	RegistObject(ev.GetType(), ev)
// }

func GetRegistType(obj interface{}) uint16 {
	rt := reflect.TypeOf(obj)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	if t, ok := registedTypes[rt]; ok {
		return t
	}
	return 0
}

func NewObjectInstance(eventType uint16) (err error, ev interface{}) {
	if t, ok := registedEvents[eventType]; !ok {
		err = errors.New("No registe event found for [" + strconv.Itoa(int(eventType)) + "]")
	} else {
		v := reflect.New(t)
		ev = v.Interface()
	}
	return
}

func NewEventInstance(eventType uint16) (err error, ev Event) {
	if t, ok := registedEvents[eventType]; !ok {
		err = errors.New("No registe event found for [" + strconv.Itoa(int(eventType)) + "]")
	} else {
		v := reflect.New(t)
		ev = v.Interface().(Event)
	}
	return
}

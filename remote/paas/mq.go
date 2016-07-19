package main

import (
	"sync"

	"github.com/yinqiwen/gsnova/common/event"
)

var queueTable map[string][]*event.EventQueue = make(map[string][]*event.EventQueue)
var queueMutex sync.Mutex

func closeUserEventQueue(user string) {
	queueMutex.Lock()
	defer queueMutex.Unlock()

	qs := queueTable[user]
	for _, q := range qs {
		if nil != q {
			q.Close()
		}
	}
	delete(queueTable, user)
}

func getEventQueue(user string, idx int, createIfMissing bool) *event.EventQueue {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	qs := queueTable[user]
	if len(qs) < (idx + 1) {
		tmp := make([]*event.EventQueue, idx+1)
		copy(tmp, qs)
		qs = tmp
	}
	q := qs[idx]
	if nil == q && createIfMissing {
		q = event.NewEventQueue()
		qs[idx] = q
		queueTable[user] = qs
	}
	return q

}

func deleteEventQueue(user string, idx int) {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	qs := queueTable[user]
	if idx < len(qs) {
		qs[idx] = nil
	}
	queueTable[user] = qs
}

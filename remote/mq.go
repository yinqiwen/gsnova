package remote

import (
	"sync"

	"github.com/yinqiwen/gsnova/common/event"
)

type UserEventQueue struct {
	runid int64
	qs    []*event.EventQueue
}

var queueTable map[string]*UserEventQueue = make(map[string]*UserEventQueue)
var queueMutex sync.Mutex

func closeUserEventQueue(user string) {
	queueMutex.Lock()
	defer queueMutex.Unlock()

	delete(queueTable, user)
}

func getUnmatchedUserRunId(cid ConnId) (int64, bool) {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	qss := queueTable[cid.User]
	if nil != qss {
		if qss.runid != cid.RunId {
			return qss.runid, true
		}
	}
	return 0, false
}

func closeUnmatchedUserEventQueue(cid ConnId) (int64, bool) {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	qss := queueTable[cid.User]
	if nil != qss {
		if qss.runid != cid.RunId {
			for _, qs := range qss.qs {
				qs.Close()
			}
			delete(queueTable, cid.User)
			return qss.runid, true
		}
	}
	return 0, false
}

func getEventQueue(cid ConnId, createIfMissing bool) (*event.EventQueue, bool) {
	qss := queueTable[cid.User]
	if nil == qss {
		if createIfMissing {
			qss = new(UserEventQueue)
			qss.runid = cid.RunId
			queueTable[cid.User] = qss
		} else {
			return nil, true
		}
	}
	if qss.runid != cid.RunId {
		return nil, false
	}
	qs := qss.qs
	if len(qs) < (cid.ConnIndex + 1) {
		tmp := make([]*event.EventQueue, cid.ConnIndex+1)
		copy(tmp, qs)
		qs = tmp
		qss.qs = qs
	}
	q := qs[cid.ConnIndex]
	if nil == q && createIfMissing {
		q = event.NewEventQueue()
		qs[cid.ConnIndex] = q
		qss.qs = qs
	}
	return q, true
}

func publishEventQueue(cid ConnId, ev event.Event) (bool, bool) {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	p, matched := getEventQueue(cid, false)
	if nil != p {
		p.Publish(ev)
		return true, true
	}
	return false, matched
}

func GetEventQueue(cid ConnId, createIfMissing bool) *event.EventQueue {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	p, _ := getEventQueue(cid, createIfMissing)
	return p
}

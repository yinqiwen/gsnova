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

	qs := queueTable[user]
	for _, q := range qs.qs {
		if nil != q {
			q.Close()
		}
	}
	delete(queueTable, user)
}

func closeUnmatchedUserEventQueue(cid ConnId) (int64, bool) {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	qss := queueTable[cid.User]
	if nil != qss {
		if qss.runid != cid.RunId {
			for _, q := range qss.qs {
				if nil != q {
					q.Close()
				}
			}
			delete(queueTable, cid.User)
			return qss.runid, true
		}
	}
	return 0, false
}

func GetEventQueue(cid ConnId, createIfMissing bool) *event.EventQueue {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	qss := queueTable[cid.User]
	if nil == qss {
		if createIfMissing {
			qss = new(UserEventQueue)
			qss.runid = cid.RunId
			queueTable[cid.User] = qss
		} else {
			return nil
		}
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
	return q
}

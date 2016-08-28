package remote

import (
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
)

type ConnEventQueue struct {
	event.EventQueue
	id         ConnId
	activeTime time.Time
	acuired    bool
}

//just for debug
func (q *ConnEventQueue) dump(wr io.Writer) {
	fmt.Fprintf(wr, "[%v]active_time=%v,acuired=%v\n", q.id, q.activeTime, q.acuired)
}

// func (q *ConnEventQueue) PeekMulti(n int, timeout time.Duration) ([]event.Event, error) {
// 	evs, err := q.EventQueue.PeekMulti(n, timeout)
// 	if nil != err {
// 		return evs, err
// 	}
// 	// for i, ev := range evs {
// 	// 	var sid SessionId
// 	// 	sid.ConnId = q.id
// 	// 	sid.Id = ev.GetId()
// 	// 	if isSessionPassiveClosed(sid) {
// 	// 		evs[i] = nil
// 	// 	}
// 	// 	if _, ok := ev.(*event.TCPCloseEvent); ok {
// 	// 		updatePassiveCloseSet(sid, false)
// 	// 	}
// 	// }
// 	return evs, nil
// }

var queueTable map[ConnId]*ConnEventQueue = make(map[ConnId]*ConnEventQueue)
var queueMutex sync.Mutex

var freeQueueTable = make(map[*ConnEventQueue]bool)
var freeQueueMutex sync.Mutex

func DumpAllQueue(wr io.Writer) {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	fmt.Fprintf(wr, "NOW=%v\n", time.Now())
	for _, queue := range queueTable {
		queue.dump(wr)
	}
}

func GetEventQueueSize() int {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	return len(queueTable)
}

func removeExpiredConnEventQueue(id ConnId) {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	delete(queueTable, id)
}

func getEventQueue(cid ConnId, createIfMissing bool) *ConnEventQueue {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	q := queueTable[cid]
	if nil == q {
		if createIfMissing {
			q = new(ConnEventQueue)
			q.EventQueue = *(event.NewEventQueue())
			q.activeTime = time.Now()
			q.id = cid
			queueTable[cid] = q
			return q
		} else {
			return nil
		}
	}
	return q
}

func GetEventQueue(cid ConnId, createIfMissing bool) *ConnEventQueue {
	q := getEventQueue(cid, createIfMissing)
	if nil != q {
		q.acuired = true
		freeQueueMutex.Lock()
		delete(freeQueueTable, q)
		freeQueueMutex.Unlock()
	}
	return q
}

func ReleaseEventQueue(q *ConnEventQueue) {
	if nil != q {
		q.acuired = false
		q.activeTime = time.Now()
		freeQueueMutex.Lock()
		freeQueueTable[q] = true
		freeQueueMutex.Unlock()
	}
}

func init() {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for {
			select {
			case <-ticker.C:
				freeQueueMutex.Lock()
				for q, _ := range freeQueueTable {
					if q.activeTime.Add(30 * time.Second).Before(time.Now()) {
						removeExpiredConnEventQueue(q.id)
						delete(freeQueueTable, q)
						log.Printf("Remove old conn event queue by id:%v", q.id)
					}
				}
				freeQueueMutex.Unlock()
			}
		}
	}()
}

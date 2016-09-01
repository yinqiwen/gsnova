package event

import (
	"errors"
	"io"
	"sync"
	"time"
)

var EventReadTimeout = errors.New("EventQueue read timeout")
var EventWriteTimeout = errors.New("EventQueue write timeout")

type EventQueue struct {
	closed bool
	mutex  sync.Mutex
	peeks  []Event
	queue  chan Event
}

func (q *EventQueue) Publish(ev Event, timeout time.Duration) error {
	//start := time.Now()
	select {
	case q.queue <- ev:
		return nil
	case <-time.After(timeout):
		return EventWriteTimeout
		// default:
		// 	if time.Now().After(start.Add(timeout)) {
		// 		return EventWriteTimeout
		// 	}
		// 	time.Sleep(1 * time.Millisecond)
	}
}
func (q *EventQueue) Close() {
	if !q.closed {
		q.closed = true
		close(q.queue)
	}
}

func (q *EventQueue) peek(timeout time.Duration) (Event, error) {
	if len(q.peeks) > 0 {
		return q.peeks[0], nil
	}
	select {
	case ev := <-q.queue:
		if nil == ev {
			return nil, io.EOF
		}
		q.peeks = []Event{ev}
		return ev, nil
	case <-time.After(timeout):
		return nil, EventReadTimeout
	}
}

func (q *EventQueue) Peek(timeout time.Duration, protect bool) (Event, error) {
	if protect {
		q.mutex.Lock()
		defer q.mutex.Unlock()
	}

	return q.peek(timeout)
}

func (q *EventQueue) PeekMulti(n int, timeout time.Duration, protect bool) ([]Event, error) {
	if protect {
		q.mutex.Lock()
		defer q.mutex.Unlock()
	}
	if len(q.peeks) > 0 {
		return q.peeks, nil
	}
	if len(q.queue) > 0 {
		for len(q.queue) > 0 && len(q.peeks) < n {
			ev := <-q.queue
			if nil != ev {
				q.peeks = append(q.peeks, ev)
			}
		}
		return q.peeks, nil
	} else {
		_, err := q.peek(timeout)
		return q.peeks, err
	}
}

func (q *EventQueue) DiscardPeeks(protect bool) {
	if protect {
		q.mutex.Lock()
		defer q.mutex.Unlock()
	}
	q.peeks = nil
}

func (q *EventQueue) ReadPeek(protect bool) Event {
	if protect {
		q.mutex.Lock()
		defer q.mutex.Unlock()
	}
	if len(q.peeks) > 0 {
		ev := q.peeks[0]
		q.peeks = q.peeks[1:]
		return ev
	}
	return nil
}

func (q *EventQueue) Read(timeout time.Duration) (Event, error) {
	select {
	case ev := <-q.queue:
		if nil == ev {
			return nil, io.EOF
		}
		return ev, nil
	case <-time.After(timeout):
		return nil, EventReadTimeout
	}
}

func NewEventQueue() *EventQueue {
	q := new(EventQueue)
	q.queue = make(chan Event, 10)
	return q
}

package event

import (
	"errors"
	"io"
	"time"
)

var EventReadTimeout = errors.New("EventQueue read timeout")

type EventQueue struct {
	closed bool
	queue  chan Event
}

func (q *EventQueue) Publish(ev Event) {
	q.queue <- ev
}
func (q *EventQueue) Close() {
	if !q.closed {
		q.closed = true
		close(q.queue)
	}
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

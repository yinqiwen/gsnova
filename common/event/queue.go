package event

import "time"

type EventQueue struct {
	queue chan Event
}

func (q *EventQueue) Publish(ev Event) {
	q.queue <- ev
}

func (q *EventQueue) Read() Event {
	select {
	case ev := <-q.queue:
		return ev
	case <-time.After(time.Second * 1):
		return nil
	}
}

func NewEventQueue() *EventQueue {
	q := new(EventQueue)
	q.queue = make(chan Event, 10)
	return q
}

package positions

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/Penlk/avito-labs/internal/domain/trip"
)

type state string

const (
	Pushing state = "running"
	Waiting state = "waiting"
)

type EnqueueResult string

const (
	Queued    EnqueueResult = "queued"
	Duplicate EnqueueResult = "duplicate"
	Dropped   EnqueueResult = "dropped"
)

type Task struct {
	trip        trip.Trip
	spanContext trace.SpanContext
	state       state
}

func (t *Task) Trip() trip.Trip { return t.trip }

func (t *Task) SpanContext() trace.SpanContext { return t.spanContext }

type Queue struct {
	queue      chan *Task
	statusTrip map[uuid.UUID]*Task
	mu         sync.Mutex
	closed     bool
}

func NewQueue(size int) (*Queue, error) {
	if size <= 0 {
		return nil, errors.New("position queue size must be positive")
	}
	return &Queue{queue: make(chan *Task, size), statusTrip: make(map[uuid.UUID]*Task)}, nil
}

func (q *Queue) TryAdd(ctx context.Context, trip trip.Trip) EnqueueResult {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return Dropped
	}
	if _, exists := q.statusTrip[trip.Id()]; exists {
		return Duplicate
	}

	task := &Task{trip: trip, spanContext: trace.SpanContextFromContext(ctx), state: Waiting}
	select {
	case q.queue <- task:
		q.statusTrip[trip.Id()] = task
		return Queued
	default:
		return Dropped
	}
}

func (q *Queue) Tasks() <-chan *Task { return q.queue }

// Len includes canceled tasks until workers receive and discard them.
func (q *Queue) Len() int { return len(q.queue) }

func (q *Queue) Start(task *Task) bool {
	if task == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.statusTrip[task.trip.Id()] != task || task.state != Waiting {
		return false
	}
	task.state = Pushing
	return true
}

func (q *Queue) Complete(task *Task) {
	if task == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.statusTrip[task.trip.Id()] == task && task.state == Pushing {
		delete(q.statusTrip, task.trip.Id())
	}
}

func (q *Queue) Cancel(tripID uuid.UUID) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.statusTrip, tripID)
}

func (q *Queue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		close(q.queue)
	}
}

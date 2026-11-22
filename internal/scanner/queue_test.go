package scanner_test

import (
	"sync"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/Penlk/avito-labs/internal/scanner"
)

func TestQueueCapacityAndCompletion(t *testing.T) {
	queue, err := scanner.NewQueue(1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(queue.Close)
	first, second := candidate(t), candidate(t)
	if queue.TryAdd(t.Context(), first) != scanner.Queued || queue.TryAdd(t.Context(), second) != scanner.Dropped {
		t.Fatal("queue capacity is not enforced")
	}
	task := <-queue.Tasks()
	if !queue.Start(task) || queue.Start(task) {
		t.Fatal("task must start exactly once")
	}
	if queue.TryAdd(t.Context(), first) != scanner.Duplicate {
		t.Fatal("in-flight trip lost deduplication")
	}
	if queue.TryAdd(t.Context(), second) != scanner.Queued {
		t.Fatal("dropped trip cannot be enqueued after capacity is freed")
	}
	queue.Complete(task)
	task = <-queue.Tasks()
	if !queue.Start(task) {
		t.Fatal("second trip cannot start")
	}
	queue.Complete(task)
	if queue.TryAdd(t.Context(), first) != scanner.Queued {
		t.Fatal("completed trip cannot be enqueued again")
	}
}

func TestQueueCancellationDoesNotAffectNewTask(t *testing.T) {
	for _, started := range []bool{false, true} {
		name := "waiting"
		if started {
			name = "in flight"
		}
		t.Run(name, func(t *testing.T) {
			queue, err := scanner.NewQueue(2)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(queue.Close)
			entity := candidate(t)
			span := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}})
			ctx := trace.ContextWithSpanContext(t.Context(), span)
			if queue.TryAdd(ctx, entity) != scanner.Queued {
				t.Fatal("first task was not queued")
			}
			old := <-queue.Tasks()
			if old.SpanContext().TraceID() != span.TraceID() || old.SpanContext().SpanID() != span.SpanID() {
				t.Fatal("task lost its trace context")
			}
			if started && !queue.Start(old) {
				t.Fatal("first task did not start")
			}
			queue.Cancel(entity.Id())
			if queue.TryAdd(ctx, entity) != scanner.Queued || queue.Start(old) {
				t.Fatal("canceled task was confused with a new task for the same trip")
			}
			current := <-queue.Tasks()
			if !queue.Start(current) {
				t.Fatal("new task did not start")
			}
			queue.Complete(old)
			if queue.TryAdd(ctx, entity) != scanner.Duplicate {
				t.Fatal("completion of old task removed new task")
			}
			queue.Complete(current)
			if queue.TryAdd(ctx, entity) != scanner.Queued {
				t.Fatal("completed task remained in deduplication state")
			}
		})
	}
}

func TestQueueConcurrentDeduplication(t *testing.T) {
	queue, err := scanner.NewQueue(2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(queue.Close)
	entity := candidate(t)
	results := make(chan scanner.EnqueueResult, 32)
	var workers sync.WaitGroup
	for range cap(results) {
		workers.Go(func() { results <- queue.TryAdd(t.Context(), entity) })
	}
	workers.Wait()
	close(results)
	counts := make(map[scanner.EnqueueResult]int)
	for result := range results {
		counts[result]++
	}
	if counts[scanner.Queued] != 1 || counts[scanner.Duplicate] != cap(results)-1 || queue.Len() != 1 {
		t.Fatalf("concurrent results=%v queue length=%d", counts, queue.Len())
	}
}

func TestQueueClose(t *testing.T) {
	queue, err := scanner.NewQueue(1)
	if err != nil {
		t.Fatal(err)
	}
	entity := candidate(t)
	if queue.TryAdd(t.Context(), entity) != scanner.Queued {
		t.Fatal("task was not queued")
	}
	queue.Close()
	queue.Close()
	if queue.TryAdd(t.Context(), candidate(t)) != scanner.Dropped {
		t.Fatal("closed queue accepted a new task")
	}
	task, open := <-queue.Tasks()
	if !open || !queue.Start(task) {
		t.Fatal("closed queue lost a pending task")
	}
	queue.Complete(task)
	if _, open := <-queue.Tasks(); open {
		t.Fatal("queue is still open after draining")
	}
}

func TestQueueRejectsInvalidSize(t *testing.T) {
	for _, size := range []int{0, -1} {
		if _, err := scanner.NewQueue(size); err == nil {
			t.Fatalf("invalid size %d was accepted", size)
		}
	}
}

package worker

import (
	"log"
	"sync"
)

// Task is a function to be executed asynchronously by a worker.
type Task func()

// Queue is a FIFO channel-backed task queue consumed by worker goroutines.
type Queue struct {
	tasks   chan Task
	wg      sync.WaitGroup
	mu      sync.RWMutex
	stopped bool
}

// New creates a Queue with the given channel buffer size.
func New(bufferSize int) *Queue {
	return &Queue{tasks: make(chan Task, bufferSize)}
}

// Submit enqueues a task. Blocks if the queue is full. Tasks submitted after
// Stop are rejected and logged rather than enqueued.
func (q *Queue) Submit(t Task) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.stopped {
		log.Print("worker: queue stopped, task rejected")
		return
	}
	q.tasks <- t
}

// Start launches n worker goroutines that consume and execute tasks.
func (q *Queue) Start(n int) {
	log.Printf("worker: starting %d workers", n)
	for i := range n {
		q.wg.Add(1)
		go func(id int) {
			defer q.wg.Done()
			for task := range q.tasks {
				task()
			}
		}(i)
	}
}

// Stop rejects new tasks, then waits for the queued and in-flight ones to
// finish. The write lock is what makes closing the channel safe: it is only
// taken once every in-flight Submit has released its read lock.
func (q *Queue) Stop() {
	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return
	}
	q.stopped = true
	close(q.tasks)
	q.mu.Unlock()

	q.wg.Wait()
}

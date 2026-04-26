package worker

import "log"

// Task is a function to be executed asynchronously by a worker.
type Task func()

// Queue is a FIFO channel-backed task queue consumed by worker goroutines.
type Queue struct {
	tasks chan Task
}

// New creates a Queue with the given channel buffer size.
func New(bufferSize int) *Queue {
	return &Queue{tasks: make(chan Task, bufferSize)}
}

// Submit enqueues a task. Blocks if the queue is full.
func (q *Queue) Submit(t Task) {
	q.tasks <- t
}

// Start launches n worker goroutines that consume and execute tasks.
func (q *Queue) Start(n int) {
	log.Printf("worker: starting %d workers", n)
	for i := range n {
		go func(id int) {
			for task := range q.tasks {
				task()
			}
		}(i)
	}
}

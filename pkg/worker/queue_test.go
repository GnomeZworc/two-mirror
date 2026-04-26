package worker

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew_ReturnsQueue(t *testing.T) {
	q := New(10)
	if q == nil {
		t.Fatal("New devrait retourner une queue non-nil")
	}
}

func TestQueue_SingleTaskExecuted(t *testing.T) {
	q := New(1)
	q.Start(1)

	var done atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	q.Submit(func() {
		done.Store(true)
		wg.Done()
	})

	wg.Wait()
	if !done.Load() {
		t.Error("la tâche n'a pas été exécutée")
	}
}

func TestQueue_AllTasksExecuted(t *testing.T) {
	const n = 50
	q := New(n)
	q.Start(1)

	var count atomic.Int32
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		q.Submit(func() {
			count.Add(1)
			wg.Done()
		})
	}

	wg.Wait()
	if count.Load() != n {
		t.Errorf("attendu %d exécutions, obtenu %d", n, count.Load())
	}
}

func TestQueue_MultipleWorkers(t *testing.T) {
	const n = 100
	q := New(n)
	q.Start(4)

	var count atomic.Int32
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		q.Submit(func() {
			count.Add(1)
			wg.Done()
		})
	}

	wg.Wait()
	if count.Load() != n {
		t.Errorf("attendu %d exécutions, obtenu %d", n, count.Load())
	}
}

func TestQueue_SubmitBlocksWhenFull(t *testing.T) {
	q := New(1)
	// Remplit le buffer sans worker
	q.Submit(func() {})

	submitted := make(chan struct{})
	go func() {
		q.Submit(func() {}) // doit bloquer jusqu'à ce qu'un worker consomme
		close(submitted)
	}()

	select {
	case <-submitted:
		t.Error("Submit aurait dû bloquer sur une queue pleine")
	case <-time.After(50 * time.Millisecond):
		// comportement attendu : goroutine bloquée
	}

	// Démarre un worker pour débloquer
	q.Start(1)
	select {
	case <-submitted:
		// Submit a pu avancer
	case <-time.After(time.Second):
		t.Error("Submit aurait dû se débloquer après démarrage d'un worker")
	}
}

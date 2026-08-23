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

func TestStop_AttendLesTachesEnCours(t *testing.T) {
	q := New(10)
	q.Start(2)

	var mu sync.Mutex
	done := 0
	for range 5 {
		q.Submit(func() {
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			done++
			mu.Unlock()
		})
	}

	q.Stop()

	mu.Lock()
	defer mu.Unlock()
	if done != 5 {
		t.Errorf("Stop devrait attendre les 5 tâches, %d terminées", done)
	}
}

func TestStop_RejetteLesTachesSuivantes(t *testing.T) {
	q := New(10)
	q.Start(1)
	q.Stop()

	executed := make(chan struct{}, 1)
	q.Submit(func() { executed <- struct{}{} })

	select {
	case <-executed:
		t.Error("une tâche soumise après Stop ne doit pas être exécutée")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestStop_Idempotent(t *testing.T) {
	q := New(10)
	q.Start(1)
	q.Stop()
	q.Stop()
}

func TestStop_SansTacheEnCours(t *testing.T) {
	q := New(10)
	q.Start(3)

	done := make(chan struct{})
	go func() { q.Stop(); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop n'a pas rendu la main sur une file vide")
	}
}

func TestSubmit_ConcurrentAvecStop(t *testing.T) {
	q := New(100)
	q.Start(4)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Submit(func() {})
		}()
	}
	q.Stop()
	wg.Wait()
}

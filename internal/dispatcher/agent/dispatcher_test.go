package dispatcher

import (
	"errors"
	"sync"
	"testing"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/state"
	"github.com/dgraph-io/badger/v4"
)

func TestDispatcher_Prepare_Success(t *testing.T) {
	d, _ := newTestDispatcher(t)
	cmd := mockCmd{
		prepareFn: func(*badger.DB, *configuration.Config) error { return nil },
		executeFn: func(*badger.DB, *configuration.Config) error { return nil },
	}
	if err := d.Prepare(cmd); err != nil {
		t.Errorf("Prepare devrait retourner nil, obtenu : %v", err)
	}
}

func TestDispatcher_Prepare_PropagatesError(t *testing.T) {
	d, _ := newTestDispatcher(t)
	want := errors.New("prepare failed")
	cmd := mockCmd{
		prepareFn: func(*badger.DB, *configuration.Config) error { return want },
		executeFn: func(*badger.DB, *configuration.Config) error { return nil },
	}
	if err := d.Prepare(cmd); !errors.Is(err, want) {
		t.Errorf("attendu %v, obtenu %v", want, err)
	}
}

func TestDispatcher_Dispatch_ExecutesCommand(t *testing.T) {
	d, _ := newTestDispatcher(t)
	var wg sync.WaitGroup
	wg.Add(1)
	cmd := mockCmd{
		prepareFn: func(*badger.DB, *configuration.Config) error { return nil },
		executeFn: func(*badger.DB, *configuration.Config) error {
			wg.Done()
			return nil
		},
	}
	d.Dispatch(cmd)
	wg.Wait()
}

func TestDispatcher_Dispatch_ExecuteErrorLogged(t *testing.T) {
	d, _ := newTestDispatcher(t)
	var wg sync.WaitGroup
	wg.Add(1)
	cmd := mockCmd{
		prepareFn: func(*badger.DB, *configuration.Config) error { return nil },
		executeFn: func(*badger.DB, *configuration.Config) error {
			defer wg.Done()
			return errors.New("execute failed")
		},
	}
	d.Dispatch(cmd)
	wg.Wait() // Execute s'est terminé — l'erreur est loggée, pas propagée
}

func TestDispatcher_Dispatch_ExecuteErrorSetsErrorState(t *testing.T) {
	d, db := newTestDispatcher(t)
	if err := state.Set(db, "vpc/vpc-1", state.Creating); err != nil {
		t.Fatalf("préparation du test : %v", err)
	}

	done := make(chan struct{})
	cmd := mockCmd{
		key:       "vpc/vpc-1",
		prepareFn: func(*badger.DB, *configuration.Config) error { return nil },
		executeFn: func(*badger.DB, *configuration.Config) error {
			return errors.New("execute failed")
		},
	}
	d.Dispatch(cmd)

	// L'état est écrit après le retour d'Execute : on scrute la DB.
	go func() {
		defer close(done)
		for {
			if s, err := state.Get(db, "vpc/vpc-1"); err == nil && s == state.Error {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		s, _ := state.Get(db, "vpc/vpc-1")
		t.Fatalf("la ressource devrait être en %q, obtenu %q", state.Error, s)
	}
}

func TestDispatcher_Dispatch_ExecuteSuccessKeepsState(t *testing.T) {
	d, db := newTestDispatcher(t)
	if err := state.Set(db, "vpc/vpc-1", state.Running); err != nil {
		t.Fatalf("préparation du test : %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	cmd := mockCmd{
		key:       "vpc/vpc-1",
		prepareFn: func(*badger.DB, *configuration.Config) error { return nil },
		executeFn: func(*badger.DB, *configuration.Config) error {
			defer wg.Done()
			return nil
		},
	}
	d.Dispatch(cmd)
	wg.Wait()
	time.Sleep(50 * time.Millisecond) // laisse le temps d'une écriture parasite

	s, err := state.Get(db, "vpc/vpc-1")
	if err != nil {
		t.Fatalf("Get a échoué : %v", err)
	}
	if s != state.Running {
		t.Errorf("l'état devrait rester %q, obtenu %q", state.Running, s)
	}
}

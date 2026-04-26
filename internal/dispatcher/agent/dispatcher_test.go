package dispatcher

import (
	"errors"
	"sync"
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
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

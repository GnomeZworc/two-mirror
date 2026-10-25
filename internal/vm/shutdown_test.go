package vm

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeQEMU struct {
	mu        sync.Mutex
	calls     []string
	dieAfter  int
	powerdown int
	dead      bool
}

func (f *fakeQEMU) send(_ string, commands []string) ([]json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := "ping"
	if len(commands) > 0 {
		call = commands[0]
	}
	f.calls = append(f.calls, call)
	if f.dead {
		return nil, errors.New("qmp dial: connection refused")
	}
	switch call {
	case qmpPowerdown:
		f.powerdown++
		if f.dieAfter > 0 && f.powerdown >= f.dieAfter {
			f.dead = true
		}
	case qmpQuit:
		f.dead = true
	}
	return nil, nil
}

func (f *fakeQEMU) count(call string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == call {
			n++
		}
	}
	return n
}

func TestShutdownQEMU_ResendsPowerdownUntilTheGuestListens(t *testing.T) {
	f := &fakeQEMU{dieAfter: 3}
	start := time.Now()
	shutdownQEMU(f.send, "qmp.sock", 5*time.Second, 50*time.Millisecond, 10*time.Millisecond)
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("arrêt en %v : le renvoi aurait dû l'obtenir bien avant le délai", d)
	}
	if n := f.count(qmpPowerdown); n != 3 {
		t.Errorf("%d system_powerdown envoyés, attendu 3 (le troisième est entendu)", n)
	}
	if n := f.count(qmpQuit); n != 0 {
		t.Errorf("quit envoyé alors que l'invité s'est arrêté : %q", f.calls)
	}
}

func TestShutdownQEMU_QuitsAtTheDeadline(t *testing.T) {
	f := &fakeQEMU{}
	start := time.Now()
	shutdownQEMU(f.send, "qmp.sock", 200*time.Millisecond, 60*time.Millisecond, 10*time.Millisecond)
	d := time.Since(start)
	if d < 200*time.Millisecond || d > 2*time.Second {
		t.Errorf("arrêt en %v, attendu au délai de 200ms", d)
	}
	if n := f.count(qmpPowerdown); n < 3 {
		t.Errorf("%d system_powerdown envoyés en 200ms avec un renvoi toutes les 60ms", n)
	}
	if n := f.count(qmpQuit); n != 1 {
		t.Errorf("%d quit envoyés, attendu 1 : %q", n, f.calls)
	}
}

func TestShutdownQEMU_StopsWhenTheGuestShutsDownOnItsOwn(t *testing.T) {
	f := &fakeQEMU{dieAfter: 1}
	shutdownQEMU(f.send, "qmp.sock", 5*time.Second, time.Second, 10*time.Millisecond)
	if n := f.count(qmpPowerdown); n != 1 {
		t.Errorf("%d system_powerdown envoyés, attendu 1", n)
	}
	if n := f.count(qmpQuit); n != 0 {
		t.Errorf("quit envoyé : %q", f.calls)
	}
}

func TestShutdownQEMU_NothingMoreWhenQEMUIsAlreadyDead(t *testing.T) {
	f := &fakeQEMU{dead: true}
	shutdownQEMU(f.send, "qmp.sock", 5*time.Second, time.Second, 10*time.Millisecond)
	if len(f.calls) != 1 {
		t.Errorf("appels QMP : %q, attendu un seul essai", f.calls)
	}
}

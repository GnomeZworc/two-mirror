package watchdog

import (
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

type notification struct {
	kind    string
	name    string
	problem string
}

type recorder struct {
	calls []notification
}

var _ notify.Notifier = (*recorder)(nil)

func (r *recorder) Notify(kind, name, problem string) {
	r.calls = append(r.calls, notification{kind: kind, name: name, problem: problem})
}

func (r *recorder) forName(name string) []notification {
	var out []notification
	for _, c := range r.calls {
		if c.name == name {
			out = append(out, c)
		}
	}
	return out
}

func (r *recorder) hasProblemContaining(substr string) bool {
	for _, c := range r.calls {
		if strings.Contains(c.problem, substr) {
			return true
		}
	}
	return false
}

func newTestDB(t *testing.T) *badger.DB {
	t.Helper()
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	return db
}

func seedResource(t *testing.T, db *badger.DB, prefix, name string, s state.State) {
	t.Helper()
	if err := state.Set(db, prefix+name, s); err != nil {
		t.Fatalf("seedResource %s%s: %v", prefix, name, err)
	}
}

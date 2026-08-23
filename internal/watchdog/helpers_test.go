package watchdog

import (
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"git.g3e.fr/syonad/two/pkg/systemd"

	"github.com/dgraph-io/badger/v4"
)

func seedKV(t *testing.T, db *badger.DB, key, value string) {
	t.Helper()
	if err := kv.AddInDB(db, key, value); err != nil {
		t.Fatalf("seedKV %s: %v", key, err)
	}
}

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

type fakeUnits struct {
	status map[string]*systemd.ServiceStatus
	err    map[string]error
	asked  []string
}

var _ unitChecker = (*fakeUnits)(nil)

func newFakeUnits() *fakeUnits {
	return &fakeUnits{
		status: map[string]*systemd.ServiceStatus{},
		err:    map[string]error{},
	}
}

func (f *fakeUnits) active(unit string) *fakeUnits {
	f.status[unit] = &systemd.ServiceStatus{Name: unit, LoadState: "loaded", ActiveState: "active", SubState: "running"}
	return f
}

func (f *fakeUnits) inactive(unit, sub string) *fakeUnits {
	f.status[unit] = &systemd.ServiceStatus{Name: unit, LoadState: "loaded", ActiveState: "inactive", SubState: sub}
	return f
}

func (f *fakeUnits) failing(unit string, err error) *fakeUnits {
	f.err[unit] = err
	return f
}

func (f *fakeUnits) Status(unit string) (*systemd.ServiceStatus, error) {
	f.asked = append(f.asked, unit)
	if err, ok := f.err[unit]; ok {
		return nil, err
	}
	if st, ok := f.status[unit]; ok {
		return st, nil
	}
	return &systemd.ServiceStatus{Name: unit, LoadState: "not-found", ActiveState: "inactive", SubState: "dead"}, nil
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

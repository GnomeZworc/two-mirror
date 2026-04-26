package dispatcher

import (
	"io"
	"log/slog"
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"git.g3e.fr/syonad/two/pkg/worker"
	"github.com/dgraph-io/badger/v4"
)

func newTestDispatcher(t *testing.T) (*Dispatcher, *badger.DB) {
	t.Helper()
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	q := worker.New(100)
	q.Start(2)
	cfg := &configuration.Config{DefaultInterface: "br-default"}
	cfg.Interfaces = map[string]string{"vms": "br-vms"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(q, db, cfg, logger), db
}

// mockCmd implémente Command sans aucune dépendance système.
type mockCmd struct {
	prepareFn func(*badger.DB, *configuration.Config) error
	executeFn func(*badger.DB, *configuration.Config) error
}

func (m mockCmd) Prepare(db *badger.DB, cfg *configuration.Config) error {
	return m.prepareFn(db, cfg)
}

func (m mockCmd) Execute(db *badger.DB, cfg *configuration.Config) error {
	return m.executeFn(db, cfg)
}

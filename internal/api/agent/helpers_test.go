package agentapi

import (
	"io"
	"log/slog"
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"git.g3e.fr/syonad/two/pkg/worker"
	"github.com/dgraph-io/badger/v4"
)

// newTestServer builds a Server backed by an in-memory Badger DB.
// The worker queue is buffered but has no running goroutines: Dispatch enqueues
// without blocking and Execute never runs, so DB state reflects only Prepare writes.
func newTestServer(t *testing.T) (*Server, *badger.DB) {
	t.Helper()
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	q := worker.New(100)
	cfg := &configuration.Config{DefaultInterface: "br-test"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := dispatcher.New(q, db, cfg, logger)
	return New(d, db, logger), db
}

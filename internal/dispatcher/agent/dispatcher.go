package dispatcher

import (
	"fmt"
	"log/slog"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/pkg/worker"
	"github.com/dgraph-io/badger/v4"
)

type Command interface {
	Prepare(db *badger.DB, cfg *configuration.Config) error
	Execute(db *badger.DB, cfg *configuration.Config) error
}

type Dispatcher struct {
	queue  *worker.Queue
	db     *badger.DB
	cfg    *configuration.Config
	logger *slog.Logger
}

func New(queue *worker.Queue, db *badger.DB, cfg *configuration.Config, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{queue: queue, db: db, cfg: cfg, logger: logger}
}

func (d *Dispatcher) Prepare(cmd Command) error {
	d.logger.Debug("prepare", "command", fmt.Sprintf("%T", cmd))
	return cmd.Prepare(d.db, d.cfg)
}

func (d *Dispatcher) Dispatch(cmd Command) {
	cmdType := fmt.Sprintf("%T", cmd)
	d.logger.Debug("dispatch", "command", cmdType)
	d.queue.Submit(func() {
		start := time.Now()
		err := cmd.Execute(d.db, d.cfg)
		attrs := []any{
			"command", cmdType,
			"duration_ms", time.Since(start).Milliseconds(),
		}
		if err != nil {
			d.logger.Error("command failed", append(attrs, "error", err)...)
		} else {
			d.logger.Info("command done", attrs...)
		}
	})
}

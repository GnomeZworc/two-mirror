package dispatcher

import (
	"log"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/pkg/worker"
	"github.com/dgraph-io/badger/v4"
)

type Command interface {
	Prepare(db *badger.DB, cfg *configuration.Config) error
	Execute(db *badger.DB, cfg *configuration.Config) error
}

type Dispatcher struct {
	queue *worker.Queue
	db    *badger.DB
	cfg   *configuration.Config
}

func New(queue *worker.Queue, db *badger.DB, cfg *configuration.Config) *Dispatcher {
	return &Dispatcher{queue: queue, db: db, cfg: cfg}
}

func (d *Dispatcher) Prepare(cmd Command) error {
	return cmd.Prepare(d.db, d.cfg)
}

func (d *Dispatcher) Dispatch(cmd Command) {
	d.queue.Submit(func() {
		if err := cmd.Execute(d.db, d.cfg); err != nil {
			log.Printf("command error (%T): %v", cmd, err)
		}
	})
}

package dispatcher

import (
	"log"

	"git.g3e.fr/syonad/two/pkg/worker"
	"github.com/dgraph-io/badger/v4"
)

type Command interface {
	Execute(db *badger.DB) error
}

type Dispatcher struct {
	queue *worker.Queue
	db    *badger.DB
}

func New(queue *worker.Queue, db *badger.DB) *Dispatcher {
	return &Dispatcher{queue: queue, db: db}
}

func (d *Dispatcher) Dispatch(cmd Command) {
	d.queue.Submit(func() {
		if err := cmd.Execute(d.db); err != nil {
			log.Printf("command error (%T): %v", cmd, err)
		}
	})
}

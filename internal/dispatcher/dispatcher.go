package dispatcher

import (
	"log"

	"git.g3e.fr/syonad/two/pkg/worker"
	"github.com/dgraph-io/badger/v4"
)

type Command interface {
	Execute(db *badger.DB, interfaces map[string]string) error
}

type Dispatcher struct {
	queue      *worker.Queue
	db         *badger.DB
	interfaces map[string]string
}

func New(queue *worker.Queue, db *badger.DB, interfaces map[string]string) *Dispatcher {
	return &Dispatcher{queue: queue, db: db, interfaces: interfaces}
}

func (d *Dispatcher) Dispatch(cmd Command) {
	d.queue.Submit(func() {
		if err := cmd.Execute(d.db, d.interfaces); err != nil {
			log.Printf("command error (%T): %v", cmd, err)
		}
	})
}

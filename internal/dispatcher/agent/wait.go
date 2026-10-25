package dispatcher

import (
	"errors"
	"time"
)

var errWaitTimeout = errors.New("timed out")

func waitUntil(timeout, poll time.Duration, done func() (bool, error)) error {
	deadline := time.After(timeout)
	for {
		ok, err := done()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		select {
		case <-deadline:
			return errWaitTimeout
		case <-time.After(poll):
		}
	}
}

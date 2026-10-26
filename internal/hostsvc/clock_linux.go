//go:build linux

package hostsvc

import (
	"time"

	"golang.org/x/sys/unix"
)

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

func (SystemClock) Sync() (bool, time.Duration, error) {
	var tx unix.Timex
	state, err := unix.Adjtimex(&tx)
	if err != nil {
		return false, 0, err
	}
	synced := state != unix.TIME_ERROR && tx.Status&unix.STA_UNSYNC == 0
	return synced, time.Duration(tx.Maxerror) * time.Microsecond, nil
}

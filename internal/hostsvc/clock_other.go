//go:build !linux

package hostsvc

import "time"

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

func (SystemClock) Sync() (bool, time.Duration, error) { return false, 0, nil }

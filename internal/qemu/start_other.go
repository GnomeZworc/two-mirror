//go:build !linux

package qemu

import "errors"

type Config struct {
	Name, Mac, VolumePath string
	TapID, Memory, CPUs   int
}

func Start(cfg Config) error {
	return errors.New("vm: not supported on this platform")
}

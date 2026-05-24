//go:build !linux

package qemu

import (
	"errors"
)

func Start(_ Config) error {
	return errors.New("vm: not supported on this platform")
}

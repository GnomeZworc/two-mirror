//go:build linux

package netns

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func call(name string, fn func() error) error {
	// entrer dans le netns cible
	f, err := os.Open(fmt.Sprintf("/var/run/netns/%s", name))
	if err != nil {
		return err
	}
	defer f.Close()

	return onThread(func() error {
		return unix.Setns(int(f.Fd()), unix.CLONE_NEWNET)
	}, fn)
}

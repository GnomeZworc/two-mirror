//go:build linux

package netns

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func enter(name string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	path := fmt.Sprintf("/var/run/netns/%s", name)

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return unix.Setns(int(f.Fd()), unix.CLONE_NEWNET)
}

//go:build linux

package netns

import (
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func onThread(enter func() error, fn func() error) error {
	runtime.LockOSThread()

	orig, err := os.Open("/proc/thread-self/ns/net")
	if err != nil {
		runtime.UnlockOSThread()
		return err
	}
	defer orig.Close()

	err = enter()
	if err == nil {
		err = fn()
	}

	if restoreErr := unix.Setns(int(orig.Fd()), unix.CLONE_NEWNET); restoreErr != nil {
		return restoreErr
	}

	runtime.UnlockOSThread()
	return err
}

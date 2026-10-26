//go:build linux

package hostsvc

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

const netnsDir = "/var/run/netns"

func netnsExists(vpc string) bool {
	_, err := os.Stat(netnsDir + "/" + vpc)
	return err == nil
}

func enterNetns(vpc string, fn func() error) error {
	runtime.LockOSThread()

	orig, err := os.Open("/proc/thread-self/ns/net")
	if err != nil {
		runtime.UnlockOSThread()
		return fmt.Errorf("open the current netns: %w", err)
	}
	defer orig.Close()

	target, err := os.Open(netnsDir + "/" + vpc)
	if err != nil {
		runtime.UnlockOSThread()
		return fmt.Errorf("open netns %s: %w", vpc, err)
	}
	defer target.Close()

	if err := unix.Setns(int(target.Fd()), unix.CLONE_NEWNET); err != nil {
		runtime.UnlockOSThread()
		return fmt.Errorf("enter netns %s: %w", vpc, err)
	}

	err = fn()

	if restore := unix.Setns(int(orig.Fd()), unix.CLONE_NEWNET); restore != nil {
		return fmt.Errorf("leave netns %s, thread discarded: %w", vpc, restore)
	}
	runtime.UnlockOSThread()
	return err
}

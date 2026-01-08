//go:build linux

package netns

import (
	"os"

	"golang.org/x/sys/unix"
)

func delete(name string) error {
	path := "/var/run/netns/" + name

	if err := unix.Unmount(path, unix.MNT_DETACH); err != nil {
		return err
	}
	return os.Remove(path)
}

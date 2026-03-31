//go:build linux

package netns

import (
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func create(name string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	base := "/var/run/netns"
	path := base + "/" + name

	if err := os.MkdirAll(base, 0755); err != nil {
		return err
	}

	// fichier cible
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	f.Close()

	// sauvegarde du netns courant
	orig, err := os.Open("/proc/self/ns/net")
	if err != nil {
		return err
	}
	defer orig.Close()

	// nouveau netns
	if err := unix.Unshare(unix.CLONE_NEWNET); err != nil {
		return err
	}

	// bind mount du netns courant vers /var/run/netns/<name>
	if err := unix.Mount(
		"/proc/self/ns/net",
		path,
		"",
		unix.MS_BIND,
		"",
	); err != nil {
		return err
	}

	// revenir au netns original
	if err := unix.Setns(int(orig.Fd()), unix.CLONE_NEWNET); err != nil {
		return err
	}

	return nil
}

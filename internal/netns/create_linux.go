//go:build linux

package netns

import (
	"fmt"
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

	// si le fichier existe déjà, le démonter d'abord
	if _, err := os.Stat(path); err == nil {
		unix.Unmount(path, unix.MNT_DETACH)
		os.Remove(path)
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

	// bind mount du netns du thread courant vers /var/run/netns/<name>
	// /proc/self/ns/net pointe vers le ns du processus (thread principal),
	// pas du thread courant — il faut utiliser le tid explicitement
	threadNsPath := fmt.Sprintf("/proc/self/task/%d/ns/net", unix.Gettid())
	if err := unix.Mount(
		threadNsPath,
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

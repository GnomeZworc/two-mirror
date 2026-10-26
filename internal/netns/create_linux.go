//go:build linux

package netns

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func create(name string) error {
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

	// nouveau netns
	return onThread(func() error {
		return unix.Unshare(unix.CLONE_NEWNET)
	}, func() error {
		// bind mount du netns du thread courant vers /var/run/netns/<name>
		// /proc/self/ns/net pointe vers le ns du processus (thread principal),
		// pas du thread courant — il faut utiliser le tid explicitement
		threadNsPath := fmt.Sprintf("/proc/self/task/%d/ns/net", unix.Gettid())
		return unix.Mount(
			threadNsPath,
			path,
			"",
			unix.MS_BIND,
			"",
		)
	})
}

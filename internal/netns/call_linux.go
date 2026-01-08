//go:build linux

package netns

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func call(name string, fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// sauvegarde du netns courant
	orig, err := os.Open("/proc/self/ns/net")
	if err != nil {
		return err
	}
	defer orig.Close()

	// entrer dans le netns cible
	f, err := os.Open(fmt.Sprintf("/var/run/netns/%s", name))
	if err != nil {
		return err
	}
	defer f.Close()

	if err := unix.Setns(int(f.Fd()), unix.CLONE_NEWNET); err != nil {
		return err
	}

	// exécuter la fonction dans le netns
	err = fn()

	// toujours revenir au netns d'origine
	if restoreErr := unix.Setns(int(orig.Fd()), unix.CLONE_NEWNET); restoreErr != nil {
		return restoreErr
	}

	return err
}

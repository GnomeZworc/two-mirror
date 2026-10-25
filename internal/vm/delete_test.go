package vm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
)

func socketDirs(t *testing.T) *configuration.Config {
	t.Helper()
	root, err := os.MkdirTemp("", "sock")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	cfg := &configuration.Config{}
	cfg.QEMU.SerialDir = filepath.Join(root, "serial")
	cfg.QEMU.MonitorDir = filepath.Join(root, "monitor")
	cfg.QEMU.QMPDir = filepath.Join(root, "qmp")
	for _, dir := range []string{cfg.QEMU.SerialDir, cfg.QEMU.MonitorDir, cfg.QEMU.QMPDir} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
}

func leave(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveQEMUSockets_RemovesTheThreeSocketsOfTheVM(t *testing.T) {
	cfg := socketDirs(t)
	sockets := []string{
		filepath.Join(cfg.QEMU.SerialDir, "t12-err.sock"),
		filepath.Join(cfg.QEMU.MonitorDir, "t12-err.sock"),
		filepath.Join(cfg.QEMU.QMPDir, "t12-err.sock"),
	}
	for _, s := range sockets {
		leave(t, s)
	}
	other := filepath.Join(cfg.QEMU.MonitorDir, "t12-ok.sock")
	leave(t, other)

	if err := removeQEMUSockets(cfg, "t12-err"); err != nil {
		t.Fatal(err)
	}
	for _, s := range sockets {
		if _, err := os.Lstat(s); !os.IsNotExist(err) {
			t.Errorf("%s laissé (%v)", s, err)
		}
	}
	if _, err := os.Lstat(other); err != nil {
		t.Errorf("socket d'une autre VM retiré : %v", err)
	}
}

func TestRemoveQEMUSockets_AbsentSocketsAreNotAnError(t *testing.T) {
	cfg := socketDirs(t)
	leave(t, filepath.Join(cfg.QEMU.MonitorDir, "t12-err.sock"))
	if err := removeQEMUSockets(cfg, "t12-err"); err != nil {
		t.Errorf("sockets serial et qmp absents : %v", err)
	}
	if err := removeQEMUSockets(cfg, "t12-err"); err != nil {
		t.Errorf("second passage : %v", err)
	}
}

func TestRemoveQEMUSockets_ReportsAFailedRemoval(t *testing.T) {
	cfg := socketDirs(t)
	blocked := filepath.Join(cfg.QEMU.QMPDir, "t12-err.sock")
	if err := os.Mkdir(blocked, 0755); err != nil {
		t.Fatal(err)
	}
	leave(t, filepath.Join(blocked, "x"))
	err := removeQEMUSockets(cfg, "t12-err")
	if err == nil || !strings.Contains(err.Error(), "remove qemu socket") || !strings.Contains(err.Error(), blocked) {
		t.Errorf("erreur %v, attendu l'échec de suppression de %s", err, blocked)
	}
}

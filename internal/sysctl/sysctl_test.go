package sysctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "net", "ipv4"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "net", "ipv4", "ip_forward"), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := root
	root = dir
	t.Cleanup(func() { root = old })
	return dir
}

func TestSetWritesTheValueAtTheKeyPath(t *testing.T) {
	dir := fakeRoot(t)
	if err := Set("net.ipv4.ip_forward", "1"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "net", "ipv4", "ip_forward"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "1" {
		t.Errorf("contenu %q, \"1\" attendu", got)
	}
}

func TestSetRefusesAnUnknownKeyWithoutCreatingIt(t *testing.T) {
	dir := fakeRoot(t)
	err := Set("net.ipv4.ip_forwarding", "1")
	if err == nil || !strings.Contains(err.Error(), "sysctl net.ipv4.ip_forwarding") {
		t.Errorf("erreur %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "net", "ipv4", "ip_forwarding")); !os.IsNotExist(err) {
		t.Errorf("clé inconnue créée : %v", err)
	}
}

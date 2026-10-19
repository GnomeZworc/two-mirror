package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	dhcpapi "git.g3e.fr/syonad/two/internal/api/dhcp"
	"git.g3e.fr/syonad/two/internal/dhcpd"
)

func startServe(t *testing.T, ctx context.Context) (string, net.PacketConn, <-chan error) {
	t.Helper()

	dir, err := os.MkdirTemp("", "dhcp")
	if err != nil {
		t.Fatalf("MkdirTemp : %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := dhcpd.NewStore(filepath.Join(dir, "state.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("Load : %v", err)
	}

	socket := filepath.Join(dir, "ctl.sock")
	control, err := dhcpapi.Listen(store, socket, log)
	if err != nil {
		t.Fatalf("Listen : %v", err)
	}
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket : %v", err)
	}
	t.Cleanup(func() {
		control.Close()
		conn.Close()
	})

	done := make(chan error, 1)
	go func() { done <- serve(ctx, control, conn, store, log) }()
	return socket, conn, done
}

func waitServe(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("serve ne s'est pas arrêté")
		return nil
	}
}

func TestServe_ArretDemandeFermeLesSockets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	socket, conn, done := startServe(t, ctx)

	if _, err := os.Stat(socket); err != nil {
		t.Fatalf("socket de contrôle absente au démarrage : %v", err)
	}

	cancel()
	if err := waitServe(t, done); err != nil {
		t.Errorf("arrêt demandé : %v, sortie normale attendue", err)
	}
	if _, err := os.Stat(socket); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("socket de contrôle laissée après l'arrêt (%v)", err)
	}
	if _, err := conn.WriteTo([]byte{0}, conn.LocalAddr()); !errors.Is(err, net.ErrClosed) {
		t.Errorf("socket DHCP encore ouverte après l'arrêt (%v)", err)
	}
}

func TestServe_RemonteLaPerteDeLaSocketDHCP(t *testing.T) {
	_, conn, done := startServe(t, context.Background())

	conn.Close()
	if err := waitServe(t, done); !errors.Is(err, net.ErrClosed) {
		t.Errorf("erreur = %v, net.ErrClosed attendu", err)
	}
}

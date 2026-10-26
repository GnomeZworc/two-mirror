//go:build linux

package metadata

import (
	"fmt"
	"net"
	"os"
	"testing"

	"git.g3e.fr/syonad/two/internal/netns"

	"golang.org/x/sys/unix"
)

type nsID struct{ dev, ino uint64 }

func statNS(t *testing.T, path string) nsID {
	t.Helper()
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		t.Fatalf("stat %s : %v", path, err)
	}
	return nsID{uint64(st.Dev), st.Ino}
}

func listenerNS(t *testing.T, ln net.Listener) nsID {
	t.Helper()
	tcp, ok := ln.(*net.TCPListener)
	if !ok {
		t.Fatalf("listener %T, *net.TCPListener attendu", ln)
	}
	raw, err := tcp.SyscallConn()
	if err != nil {
		t.Fatalf("SyscallConn : %v", err)
	}
	var id nsID
	var nsErr error
	if err := raw.Control(func(fd uintptr) {
		nsFD, err := unix.IoctlRetInt(int(fd), unix.SIOCGSKNS)
		if err != nil {
			nsErr = err
			return
		}
		defer unix.Close(nsFD)
		var st unix.Stat_t
		if nsErr = unix.Fstat(nsFD, &st); nsErr == nil {
			id = nsID{uint64(st.Dev), st.Ino}
		}
	}); err != nil {
		t.Fatalf("Control : %v", err)
	}
	if nsErr != nil {
		t.Fatalf("netns de la socket : %v", nsErr)
	}
	return id
}

func TestListen_DansLeNetnsDuVPC(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("netns : root requis")
	}
	name := fmt.Sprintf("t92-vpc-%d", os.Getpid())
	if err := netns.Create(name); err != nil {
		t.Fatalf("création de %s : %v", name, err)
	}
	t.Cleanup(func() {
		if err := netns.Delete(name); err != nil {
			t.Errorf("suppression de %s : %v", name, err)
		}
	})

	for i := 0; i < 50; i++ {
		ln, err := listen(name, ":0")
		if err != nil {
			t.Fatalf("listen : %v", err)
		}
		got := listenerNS(t, ln)
		ln.Close()
		if want := statNS(t, "/var/run/netns/"+name); got != want {
			t.Fatalf("essai %d : socket dans le netns %v, netns %s %v attendu", i, got, name, want)
		}
	}
}

func TestListen_SansNetnsDansLeNetnsCourant(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("netns : root requis")
	}
	ln, err := listen("", ":0")
	if err != nil {
		t.Fatalf("listen : %v", err)
	}
	defer ln.Close()
	if got, want := listenerNS(t, ln), statNS(t, "/proc/self/ns/net"); got != want {
		t.Errorf("socket dans le netns %v, netns du processus %v attendu", got, want)
	}
}

func TestListen_NetnsAbsent(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("netns : root requis")
	}
	ln, err := listen(fmt.Sprintf("t92-absent-%d", os.Getpid()), ":0")
	if err == nil {
		ln.Close()
		t.Fatalf("listen sans netns : aucune erreur, échec attendu")
	}
	if !os.IsNotExist(err) {
		t.Errorf("listen : %v, netns absent attendu", err)
	}
}

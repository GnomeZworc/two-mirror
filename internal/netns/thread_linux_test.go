//go:build linux

package netns

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"

	"golang.org/x/sys/unix"
)

func init() {
	runtime.LockOSThread()
}

var onMain = make(chan func())

func TestMain(m *testing.M) {
	code := make(chan int)
	go func() { code <- m.Run() }()
	for {
		select {
		case f := <-onMain:
			f()
		case c := <-code:
			os.Exit(c)
		}
	}
}

func runOnMain(f func()) {
	done := make(chan struct{})
	onMain <- func() {
		f()
		close(done)
	}
	<-done
}

type nsID struct{ dev, ino uint64 }

func statNS(t *testing.T, path string) nsID {
	t.Helper()
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		t.Fatalf("stat %s : %v", path, err)
	}
	return nsID{uint64(st.Dev), st.Ino}
}

func threadNS(t *testing.T) nsID {
	t.Helper()
	return statNS(t, "/proc/thread-self/ns/net")
}

func setupMainInOtherNS(t *testing.T) (nsID, string) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("netns : root requis")
	}
	runtime.LockOSThread()
	root := threadNS(t)
	rootFD, err := os.Open("/proc/thread-self/ns/net")
	if err != nil {
		t.Fatalf("netns racine : %v", err)
	}
	runtime.UnlockOSThread()

	other := fmt.Sprintf("t91-main-%d", os.Getpid())
	if err := Create(other); err != nil {
		t.Fatalf("création de %s : %v", other, err)
	}
	t.Cleanup(func() {
		if err := Delete(other); err != nil {
			t.Errorf("suppression de %s : %v", other, err)
		}
	})

	var setErr error
	runOnMain(func() {
		f, err := os.Open("/var/run/netns/" + other)
		if err != nil {
			setErr = err
			return
		}
		defer f.Close()
		setErr = unix.Setns(int(f.Fd()), unix.CLONE_NEWNET)
	})
	if setErr != nil {
		t.Fatalf("thread principal dans %s : %v", other, setErr)
	}
	t.Cleanup(func() {
		var err error
		runOnMain(func() { err = unix.Setns(int(rootFD.Fd()), unix.CLONE_NEWNET) })
		rootFD.Close()
		if err != nil {
			t.Fatalf("thread principal ramené au netns racine : %v", err)
		}
	})
	return root, other
}

func assertBackToRoot(t *testing.T, root nsID) {
	t.Helper()
	if got := threadNS(t); got != root {
		t.Fatalf("thread laissé dans le netns %v, netns racine %v attendu", got, root)
	}
	runtime.UnlockOSThread()
}

func TestCall_ThreadPrincipalDansUnAutreNetns(t *testing.T) {
	root, other := setupMainInOtherNS(t)
	target := fmt.Sprintf("t91-cible-%d", os.Getpid())
	if err := Create(target); err != nil {
		t.Fatalf("création de %s : %v", target, err)
	}
	t.Cleanup(func() {
		if err := Delete(target); err != nil {
			t.Errorf("suppression de %s : %v", target, err)
		}
	})

	runtime.LockOSThread()
	var inside nsID
	if err := Call(target, func() error {
		inside = threadNS(t)
		return nil
	}); err != nil {
		t.Fatalf("Call : %v", err)
	}
	if want := statNS(t, "/var/run/netns/"+target); inside != want {
		t.Errorf("fn exécutée dans %v, netns %s %v attendu", inside, target, want)
	}
	if inside == statNS(t, "/var/run/netns/"+other) {
		t.Errorf("fn exécutée dans le netns du thread principal")
	}
	assertBackToRoot(t, root)
}

func TestCall_ErreurDeFnRenvoyeeEtThreadRestaure(t *testing.T) {
	root, _ := setupMainInOtherNS(t)
	target := fmt.Sprintf("t91-err-%d", os.Getpid())
	if err := Create(target); err != nil {
		t.Fatalf("création de %s : %v", target, err)
	}
	t.Cleanup(func() {
		if err := Delete(target); err != nil {
			t.Errorf("suppression de %s : %v", target, err)
		}
	})

	runtime.LockOSThread()
	want := errors.New("échec de fn")
	if err := Call(target, func() error { return want }); !errors.Is(err, want) {
		t.Errorf("Call : %v, %v attendu", err, want)
	}
	assertBackToRoot(t, root)
}

func TestCall_NetnsAbsent(t *testing.T) {
	root, _ := setupMainInOtherNS(t)

	runtime.LockOSThread()
	called := false
	err := Call(fmt.Sprintf("t91-absent-%d", os.Getpid()), func() error {
		called = true
		return nil
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Call : %v, netns absent attendu", err)
	}
	if called {
		t.Errorf("fn appelée sans netns")
	}
	assertBackToRoot(t, root)
}

func TestCreate_ThreadPrincipalDansUnAutreNetns(t *testing.T) {
	root, other := setupMainInOtherNS(t)
	name := fmt.Sprintf("t91-cree-%d", os.Getpid())

	runtime.LockOSThread()
	if err := Create(name); err != nil {
		t.Fatalf("Create : %v", err)
	}
	t.Cleanup(func() {
		if err := Delete(name); err != nil {
			t.Errorf("suppression de %s : %v", name, err)
		}
	})
	created := statNS(t, "/var/run/netns/"+name)
	if created == root || created == statNS(t, "/var/run/netns/"+other) {
		t.Errorf("netns %s %v : nouveau netns attendu", name, created)
	}
	assertBackToRoot(t, root)
}

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/frr"
	"git.g3e.fr/syonad/two/pkg/worker"
)

type fakeServer struct {
	called atomic.Bool
	err    error
}

func (f *fakeServer) Shutdown(context.Context) error {
	f.called.Store(true)
	return f.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestShutdown_DrainReussi(t *testing.T) {
	q := worker.New(10)
	q.Start(2)

	var done atomic.Int32
	for range 3 {
		q.Submit(func() {
			time.Sleep(20 * time.Millisecond)
			done.Add(1)
		})
	}

	api, prom := &fakeServer{}, &fakeServer{}
	servers := map[string]httpShutdowner{"api": api, "prometheus": prom}

	if !shutdown(discardLogger(), q, servers, 5*time.Second) {
		t.Fatal("un drainage réussi doit autoriser la fermeture de la base")
	}
	if !api.called.Load() || !prom.called.Load() {
		t.Error("tous les serveurs HTTP doivent être arrêtés")
	}
	if got := done.Load(); got != 3 {
		t.Errorf("les 3 tâches devaient se terminer, %d terminées", got)
	}
}

func TestShutdown_TimeoutLaisseLaBaseIntacte(t *testing.T) {
	q := worker.New(10)
	q.Start(1)
	q.Submit(func() { time.Sleep(2 * time.Second) })

	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, nil))

	if shutdown(log, q, map[string]httpShutdowner{}, 50*time.Millisecond) {
		t.Fatal("un drainage incomplet ne doit pas autoriser la fermeture de la base")
	}
	if !strings.Contains(buf.String(), "leaving database untouched") {
		t.Errorf("le dépassement devrait être logué, obtenu %q", buf.String())
	}
}

func TestShutdown_ErreurServeurNEmpechePasLeDrainage(t *testing.T) {
	q := worker.New(10)
	q.Start(1)

	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, nil))
	servers := map[string]httpShutdowner{
		"api":        &fakeServer{err: errors.New("boom")},
		"prometheus": &fakeServer{},
	}

	if !shutdown(log, q, servers, 5*time.Second) {
		t.Fatal("une erreur d'arrêt HTTP ne doit pas empêcher le drainage")
	}
	if !strings.Contains(buf.String(), "http server shutdown") {
		t.Errorf("l'erreur devrait être loguée, obtenu %q", buf.String())
	}
}

func TestShutdown_SansServeur(t *testing.T) {
	q := worker.New(10)
	q.Start(1)

	if !shutdown(discardLogger(), q, map[string]httpShutdowner{}, 5*time.Second) {
		t.Fatal("l'absence de serveur ne doit pas empêcher un arrêt propre")
	}
}

type fakeBase struct{ called bool }

func (f *fakeBase) ApplyBase(frr.Base) error {
	f.called = true
	return nil
}

func TestApplyBGPBase_RefusesAnInvalidSectionBeforeTouchingFRR(t *testing.T) {
	cfg := &configuration.Config{}
	cfg.BGP.RouteReflectors = []string{"10.255.255.1"}
	f := &fakeBase{}
	if err := applyBGPBase(cfg, f); err == nil || !strings.Contains(err.Error(), "bgp.asn must be set") {
		t.Errorf("erreur %v", err)
	}
	if f.called {
		t.Error("ApplyBase appelé avec une section bgp invalide")
	}
}

func TestRunExitsNonZeroWhenTheConfigCannotBeLoaded(t *testing.T) {
	conf := filepath.Join(t.TempDir(), "agent.yml")
	if err := os.WriteFile(conf, []byte("bgp: [non: valide\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := run(conf); code != 1 {
		t.Errorf("code de sortie %d, 1 attendu", code)
	}
}

func TestRunExitsNonZeroWhenTheBGPBaseCannotBeApplied(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "agent.yml")
	yml := "database:\n  path: " + filepath.Join(dir, "db") + "\n" +
		"default_interface: two-test-absent0\n" +
		"bgp:\n  asn: 64600\n  route_reflectors: [10.255.255.1]\n"
	if err := os.WriteFile(conf, []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := run(conf); code != 1 {
		t.Errorf("code de sortie %d, 1 attendu", code)
	}
}

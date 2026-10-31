package host

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestCollector_RegisterServesTheFactsReadAtCreation(t *testing.T) {
	fsys := debian12()
	c := NewCollector(fsys, "0.3.0", time.Unix(1792210950, 0), time.Hour, discard())
	fsys["proc/sys/kernel/hostname"].Data = []byte("autre\n")
	f, err := c.Register()
	if err != nil || f.Hostname != "hv1" {
		t.Errorf("Register = %q, %v ; hv1 lu à la création attendu", f.Hostname, err)
	}
}

func TestCollector_RegisterKeepsTheReadError(t *testing.T) {
	fsys := debian12()
	delete(fsys, "proc/cpuinfo")
	var logs strings.Builder
	c := NewCollector(fsys, "0.3.0", time.Unix(0, 0), time.Hour, slog.New(slog.NewTextHandler(&logs, nil)))
	if _, err := c.Register(); err == nil {
		t.Error("Register sans cpuinfo : aucune erreur")
	}
	if !strings.Contains(logs.String(), "host facts unavailable") {
		t.Errorf("erreur non journalisée : %q", logs.String())
	}
}

func TestCollector_PingBeforeAnyCollection(t *testing.T) {
	c := NewCollector(debian12(), "0.3.0", time.Unix(0, 0), time.Hour, discard())
	if _, ok := c.Ping(); ok {
		t.Error("Ping avant toute mesure : instantané rendu")
	}
}

func TestCollector_PingServesTheLastCollection(t *testing.T) {
	c := NewCollector(debian12(), "0.3.0", time.Unix(1792210950, 0), time.Hour, discard())
	c.now = func() time.Time { return time.Unix(1793454325, 0) }
	c.collect()
	got, ok := c.Ping()
	want := Ping{
		State:          State{BootTime: 1792210880, UptimeSeconds: 1243445.41, Load: [3]float64{0.08, 0.03, 0.04}},
		AgentVersion:   "0.3.0",
		AgentStartTime: 1792210950,
		CollectedAt:    1793454325,
	}
	if !ok || got != want {
		t.Errorf("Ping = %+v, %v ; %+v attendu", got, ok, want)
	}
}

func TestCollector_FailedCollectionKeepsThePreviousOne(t *testing.T) {
	fsys := debian12()
	var logs strings.Builder
	c := NewCollector(fsys, "0.3.0", time.Unix(0, 0), time.Hour, slog.New(slog.NewTextHandler(&logs, nil)))
	c.now = func() time.Time { return time.Unix(100, 0) }
	c.collect()
	delete(fsys, "proc/loadavg")
	c.now = func() time.Time { return time.Unix(200, 0) }
	c.collect()
	got, ok := c.Ping()
	if !ok || got.CollectedAt != 100 {
		t.Errorf("Ping = %+v, %v ; mesure de 100 gardée attendue", got, ok)
	}
	if !strings.Contains(logs.String(), "host state collection failed") {
		t.Errorf("échec non journalisé : %q", logs.String())
	}
}

func TestCollector_RunCollectsAtOnceAndStopsWithTheContext(t *testing.T) {
	c := NewCollector(debian12(), "0.3.0", time.Unix(0, 0), time.Hour, discard())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := c.Ping(); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("aucune mesure au démarrage de Run")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run ne s'arrête pas à l'annulation du contexte")
	}
}

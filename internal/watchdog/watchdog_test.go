package watchdog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
)

type chanNotifier struct {
	seen chan notification
}

var _ notify.Notifier = (*chanNotifier)(nil)

func newChanNotifier() *chanNotifier {
	return &chanNotifier{seen: make(chan notification, 256)}
}

func (c *chanNotifier) Notify(kind, name, problem string) {
	select {
	case c.seen <- notification{kind: kind, name: name, problem: problem}:
	default:
	}
}

func testLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("ligne non JSON %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestNew_IntervalleInvalideUtiliseLeDefaut(t *testing.T) {
	l, buf := testLogger()

	for _, interval := range []time.Duration{0, -time.Second} {
		w := New(newTestDB(t), testCfg(t), &recorder{}, l, interval)
		if w.interval != defaultInterval {
			t.Errorf("interval = %v, attendu %v", w.interval, defaultInterval)
		}
	}

	if !strings.Contains(buf.String(), "invalid interval") {
		t.Error("un intervalle invalide devrait être signalé dans les logs")
	}
}

func TestNew_IntervalleValideConserve(t *testing.T) {
	l, _ := testLogger()
	w := New(newTestDB(t), testCfg(t), &recorder{}, l, 5*time.Second)
	if w.interval != 5*time.Second {
		t.Errorf("interval = %v, attendu 5s", w.interval)
	}
}

func TestNew_LoggerNil(t *testing.T) {
	w := New(newTestDB(t), testCfg(t), &recorder{}, nil, time.Second)
	if w.logger == nil {
		t.Fatal("logger nil non remplacé par slog.Default()")
	}
}

func TestRun_SArreteSurContexteAnnule(t *testing.T) {
	l, _ := testLogger()
	w := New(newTestDB(t), testCfg(t), &recorder{}, l, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run n'a pas rendu la main après annulation du contexte")
	}
}

func TestRun_VerifieLesTroisTypesDeRessource(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "vp-fantome", state.Running)
	seedSubnet(t, db, "br-000042", "vp-admin", modeBridge)
	seedVM(t, db, "i-test1", "br-000042", "vp-admin", "12345678")

	n := newChanNotifier()
	l, _ := testLogger()
	w := New(db, testCfg(t), n, l, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	kinds := map[string]bool{}
	deadline := time.After(5 * time.Second)
	for len(kinds) < 3 {
		select {
		case c := <-n.seen:
			kinds[c.kind] = true
		case <-deadline:
			t.Fatalf("types de ressource vérifiés: %v, attendu vpc + subnet + vm", kinds)
		}
	}
}

func TestRun_ErreurDeBaseLogueeEtBoucleContinue(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "vp-fantome", state.Running)
	db.Close()

	n := newChanNotifier()
	l, buf := testLogger()
	w := New(db, testCfg(t), n, l, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	var vpc, subnet, vm bool
	for _, line := range logLines(t, buf) {
		msg, _ := line["msg"].(string)
		switch {
		case strings.Contains(msg, "vpc check failed"):
			vpc = true
		case strings.Contains(msg, "subnet check failed"):
			subnet = true
		case strings.Contains(msg, "vm check failed"):
			vm = true
		}
	}
	if !vpc || !subnet || !vm {
		t.Errorf("les trois erreurs devraient être loguées (vpc=%v subnet=%v vm=%v)", vpc, subnet, vm)
	}
	if len(n.seen) != 0 {
		t.Errorf("une erreur d'accès à la base ne doit pas produire de notification, obtenu %d", len(n.seen))
	}
}

func TestRun_LogueDemarrageEtArret(t *testing.T) {
	l, buf := testLogger()
	w := New(newTestDB(t), testCfg(t), &recorder{}, l, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.Run(ctx)

	out := buf.String()
	if !strings.Contains(out, "watchdog: starting") {
		t.Error("le démarrage devrait être logué")
	}
	if !strings.Contains(out, "watchdog: stopping") {
		t.Error("l'arrêt devrait être logué")
	}
}

func TestUnits_ConnexionSystemdIndisponibleSignaleeUneSeuleFois(t *testing.T) {
	l, buf := testLogger()
	w := New(newTestDB(t), testCfg(t), &recorder{}, l, time.Hour)

	for range 3 {
		u, close := w.units()
		close()
		if u != nil {
			t.Skip("systemd joignable sur cette machine, cas non testable")
		}
	}

	var warnings int
	for _, line := range logLines(t, buf) {
		if msg, _ := line["msg"].(string); strings.Contains(msg, "systemd unreachable") {
			warnings++
		}
	}
	if warnings != 1 {
		t.Errorf("attendu 1 avertissement pour 3 tentatives, obtenu %d", warnings)
	}
}

package notify

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func newTestNotifier(t *testing.T) (*StderrNotifier, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewStderr(l), &buf
}

func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
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

func TestNotify_EmetLesTroisChampsStructures(t *testing.T) {
	n, buf := newTestNotifier(t)

	n.Notify("vm", "i-test1", "tap12 absente du netns vp-admin")

	lines := decodeLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("attendu 1 ligne, obtenu %d", len(lines))
	}

	for field, want := range map[string]string{
		"kind":    "vm",
		"name":    "i-test1",
		"problem": "tap12 absente du netns vp-admin",
	} {
		got, ok := lines[0][field]
		if !ok {
			t.Errorf("champ %q absent de la sortie", field)
			continue
		}
		if got != want {
			t.Errorf("champ %q = %v, attendu %q", field, got, want)
		}
	}
}

func TestNotify_NiveauError(t *testing.T) {
	n, buf := newTestNotifier(t)

	n.Notify("vpc", "vp-admin", "netns absent")

	lines := decodeLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("attendu 1 ligne, obtenu %d", len(lines))
	}
	if lines[0]["level"] != "ERROR" {
		t.Errorf("level = %v, attendu ERROR", lines[0]["level"])
	}
}

func TestNotify_MessageNeContientPasLesValeurs(t *testing.T) {
	n, buf := newTestNotifier(t)

	n.Notify("subnet", "br-000042", "bridge down")

	lines := decodeLines(t, buf)
	msg, _ := lines[0]["msg"].(string)
	for _, v := range []string{"br-000042", "bridge down"} {
		if strings.Contains(msg, v) {
			t.Errorf("msg %q ne devrait pas contenir la valeur %q (elle doit être un attribut)", msg, v)
		}
	}
}

func TestNotify_AucuneDeduplication(t *testing.T) {
	n, buf := newTestNotifier(t)

	for range 3 {
		n.Notify("vm", "i-test1", "qemu ne répond pas sur QMP")
	}

	if lines := decodeLines(t, buf); len(lines) != 3 {
		t.Errorf("attendu 3 lignes (une par appel), obtenu %d", len(lines))
	}
}

func TestNotify_ChampsVides(t *testing.T) {
	n, buf := newTestNotifier(t)

	n.Notify("", "", "")

	if lines := decodeLines(t, buf); len(lines) != 1 {
		t.Errorf("attendu 1 ligne, obtenu %d", len(lines))
	}
}

func TestNewStderr_LoggerNil(t *testing.T) {
	n := NewStderr(nil)
	if n.logger == nil {
		t.Fatal("logger nil non remplacé par slog.Default()")
	}
	n.Notify("vpc", "vp-admin", "netns absent")
}

func TestNewStderr_ImplementeNotifier(t *testing.T) {
	var _ Notifier = NewStderr(nil)
}

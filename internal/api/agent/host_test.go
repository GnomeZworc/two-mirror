package agentapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/internal/host"
)

type fakeHost struct {
	facts    host.Facts
	factsErr error
	ping     *host.Ping
}

func (f *fakeHost) Register() (host.Facts, error) { return f.facts, f.factsErr }

func (f *fakeHost) Ping() (host.Ping, bool) {
	if f.ping == nil {
		return host.Ping{}, false
	}
	return *f.ping, true
}

func serveHost(t *testing.T, h *fakeHost, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	s, _ := newTestServer(t)
	s.host = h
	w := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestRegister_ServesTheFacts(t *testing.T) {
	h := &fakeHost{facts: host.Facts{
		Hostname:    "hv1",
		CPU:         host.CPU{Vendor: "AuthenticAMD", Model: "AMD EPYC 7272 12-Core Processor", Sockets: 1, Cores: 12, Threads: 24},
		MemoryBytes: 134217728000,
		OS:          host.OS{ID: "debian", VersionID: "12", PrettyName: "Debian GNU/Linux 12 (bookworm)"},
		Kernel:      "6.1.0-26-amd64",
	}}
	w := serveHost(t, h, http.MethodGet, "/register")
	if w.Code != http.StatusOK {
		t.Fatalf("attendu 200, obtenu %d : %s", w.Code, w.Body.String())
	}
	want := `{"hostname":"hv1","cpu":{"vendor":"AuthenticAMD","model":"AMD EPYC 7272 12-Core Processor","sockets":1,"cores":12,"threads":24},"memory_bytes":134217728000,"os":{"id":"debian","version_id":"12","pretty_name":"Debian GNU/Linux 12 (bookworm)"},"kernel":"6.1.0-26-amd64"}`
	if got := strings.TrimSpace(w.Body.String()); got != want {
		t.Errorf("corps :\n%s\nattendu :\n%s", got, want)
	}
}

func TestRegister_UnavailableFacts(t *testing.T) {
	w := serveHost(t, &fakeHost{factsErr: errors.New("open proc/cpuinfo: file does not exist")}, http.MethodGet, "/register")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("attendu 503, obtenu %d", w.Code)
	}
	var e ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&e); err != nil || e.Error != "host facts unavailable: open proc/cpuinfo: file does not exist" {
		t.Errorf("erreur %q, %v", e.Error, err)
	}
}

func TestPing_ServesTheLastState(t *testing.T) {
	h := &fakeHost{ping: &host.Ping{
		State:          host.State{BootTime: 1793499123, UptimeSeconds: 21730.4, Load: [3]float64{0.12, 0.3, 0.25}},
		AgentVersion:   "0.3.0",
		AgentStartTime: 1793499190,
		CollectedAt:    1793520853,
	}}
	w := serveHost(t, h, http.MethodGet, "/ping")
	if w.Code != http.StatusOK {
		t.Fatalf("attendu 200, obtenu %d : %s", w.Code, w.Body.String())
	}
	want := `{"agent_version":"0.3.0","collected_at":1793520853,"boot_time":1793499123,"agent_start_time":1793499190,"uptime_seconds":21730.4,"load":[0.12,0.3,0.25]}`
	if got := strings.TrimSpace(w.Body.String()); got != want {
		t.Errorf("corps :\n%s\nattendu :\n%s", got, want)
	}
}

func TestPing_BeforeTheFirstCollection(t *testing.T) {
	w := serveHost(t, &fakeHost{}, http.MethodGet, "/ping")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("attendu 503, obtenu %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "no host state collected yet") {
		t.Errorf("corps %s", w.Body.String())
	}
}

func TestHostRoutes_ReadOnly(t *testing.T) {
	h := &fakeHost{ping: &host.Ping{}}
	for _, path := range []string{"/register", "/ping"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			if w := serveHost(t, h, method, path); w.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s : attendu 405, obtenu %d", method, path, w.Code)
			}
		}
	}
}

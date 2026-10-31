package agentapi

import (
	"encoding/json"
	"net/http"

	"git.g3e.fr/syonad/two/internal/host"
)

type HostInfo interface {
	Register() (host.Facts, error)
	Ping() (host.Ping, bool)
}

func (s *Server) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	f, err := s.host.Register()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "host facts unavailable: " + err.Error()})
		return
	}
	json.NewEncoder(w).Encode(HostRegister{
		Hostname:    f.Hostname,
		CPU:         HostCPU{Vendor: f.CPU.Vendor, Model: f.CPU.Model, Sockets: f.CPU.Sockets, Cores: f.CPU.Cores, Threads: f.CPU.Threads},
		MemoryBytes: f.MemoryBytes,
		OS:          HostOS{ID: f.OS.ID, VersionID: f.OS.VersionID, PrettyName: f.OS.PrettyName},
		Kernel:      f.Kernel,
	})
}

func (s *Server) PingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	p, ok := s.host.Ping()
	if !ok {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "no host state collected yet"})
		return
	}
	json.NewEncoder(w).Encode(HostPing{
		AgentVersion:   p.AgentVersion,
		CollectedAt:    p.CollectedAt,
		BootTime:       p.BootTime,
		AgentStartTime: p.AgentStartTime,
		UptimeSeconds:  p.UptimeSeconds,
		Load:           p.Load,
	})
}

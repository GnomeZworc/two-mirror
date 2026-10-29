package agentapi

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func (s *Server) FloatingIPsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		s.listFloatingIPs(w, r)
	case http.MethodPost:
		s.postFloatingIP(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) listFloatingIPs(w http.ResponseWriter, _ *http.Request) {
	entries, err := kv.ListByPrefix(s.db, "floating-ip/")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to list floating ips"})
		return
	}
	ips := make(map[string]*FloatingIP)
	for key, value := range entries {
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			continue
		}
		name := parts[1]
		if _, ok := ips[name]; !ok {
			ips[name] = &FloatingIP{Name: name}
		}
		switch parts[2] {
		case "state":
			ips[name].State = value
		case "vm":
			ips[name].VM = value
		case "interface":
			ips[name].Interface = value
		case "ip":
			ips[name].IP = value
		case "gateway":
			ips[name].Gateway = value
		}
	}
	result := make([]FloatingIP, 0, len(ips))
	for _, f := range ips {
		result = append(result, *f)
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

func (s *Server) postFloatingIP(w http.ResponseWriter, r *http.Request) {
	var req FloatingIPCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request body"})
		return
	}
	for _, f := range []struct{ name, value string }{{"name", req.Name}, {"vm", req.VM}, {"interface", req.Interface}, {"ip", req.IP}, {"gateway", req.Gateway}} {
		if f.value == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: f.name + " is required"})
			return
		}
	}
	if ip, _, err := net.ParseCIDR(req.IP); err != nil || ip.To4() == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid ip: an IPv4 address with its prefix length is expected"})
		return
	}
	if gw := net.ParseIP(req.Gateway); gw == nil || gw.To4() == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid gateway: an IPv4 address is expected"})
		return
	}
	cmd := dispatcher.CreateFloatingIPCommand{Name: req.Name, VM: req.VM, Interface: req.Interface, IP: req.IP, Gateway: req.Gateway}
	if err := s.dispatcher.Prepare(cmd); err != nil {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	s.dispatcher.Dispatch(cmd)
	state, err := kv.GetFromDB(s.db, "floating-ip/"+req.Name+"/state")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read floating ip state"})
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(FloatingIP{Name: req.Name, State: state, VM: req.VM, Interface: req.Interface, IP: req.IP, Gateway: req.Gateway})
}

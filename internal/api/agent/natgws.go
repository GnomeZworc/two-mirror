package agentapi

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func (s *Server) NATGatewaysHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		s.listNATGateways(w, r)
	case http.MethodPost:
		s.postNATGateway(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) listNATGateways(w http.ResponseWriter, _ *http.Request) {
	entries, err := kv.ListByPrefix(s.db, "nat-gateway/")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to list nat gateways"})
		return
	}
	gateways := make(map[string]*NATGateway)
	for key, value := range entries {
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			continue
		}
		name := parts[1]
		if _, ok := gateways[name]; !ok {
			gateways[name] = &NATGateway{Name: name}
		}
		switch parts[2] {
		case "state":
			gateways[name].State = value
		case "vpc":
			gateways[name].VPC = value
		case "ip":
			gateways[name].IP = value
		case "gateway":
			gateways[name].Gateway = value
		}
	}
	result := make([]NATGateway, 0, len(gateways))
	for _, g := range gateways {
		result = append(result, *g)
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

func (s *Server) postNATGateway(w http.ResponseWriter, r *http.Request) {
	var req NATGatewayCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request body"})
		return
	}
	for _, f := range []struct{ name, value string }{{"name", req.Name}, {"vpc", req.VPC}, {"ip", req.IP}, {"gateway", req.Gateway}} {
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
	cmd := dispatcher.CreateNATGatewayCommand{Name: req.Name, VPC: req.VPC, IP: req.IP, Gateway: req.Gateway}
	if err := s.dispatcher.Prepare(cmd); err != nil {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	s.dispatcher.Dispatch(cmd)
	state, err := kv.GetFromDB(s.db, "nat-gateway/"+req.Name+"/state")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read nat gateway state"})
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(NATGateway{Name: req.Name, State: state, VPC: req.VPC, IP: req.IP, Gateway: req.Gateway})
}

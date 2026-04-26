package agentapi

import (
	"encoding/json"
	"net/http"
	"strings"

	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func (s *Server) VpcsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		s.listVpcs(w, r)
	case http.MethodPost:
		s.postVpc(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) listVpcs(w http.ResponseWriter, _ *http.Request) {
	entries, err := kv.ListByPrefix(s.db, "vpc/")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to list vpcs"})
		return
	}
	vpcs := make(map[string]*VPC)
	for key, value := range entries {
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			continue
		}
		name := parts[1]
		if _, ok := vpcs[name]; !ok {
			vpcs[name] = &VPC{Name: name}
		}
		if parts[2] == "state" {
			vpcs[name].State = value
		}
	}
	result := make([]VPC, 0, len(vpcs))
	for _, v := range vpcs {
		result = append(result, *v)
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

func (s *Server) postVpc(w http.ResponseWriter, r *http.Request) {
	var req VPCCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request body"})
		return
	}
	if req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name is required"})
		return
	}
	cmd := dispatcher.CreateVPCCommand{Name: req.Name}
	if err := s.dispatcher.Prepare(cmd); err != nil {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	s.dispatcher.Dispatch(cmd)
	state, err := kv.GetFromDB(s.db, "vpc/"+req.Name+"/state")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read vpc state"})
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(VPC{Name: req.Name, State: state})
}

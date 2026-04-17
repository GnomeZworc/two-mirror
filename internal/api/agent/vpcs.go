package agentapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"git.g3e.fr/syonad/two/internal/vpc"
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
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode([]VPC{})
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
	s.queue.Submit(func() {
		kv.AddInDB(s.db, "vpc/"+req.Name+"/state", "creating")
		if err := vpc.CreateVPC(s.db, req.Name); err != nil {
			fmt.Println(err)
		}
	})
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(VPC{Name: req.Name, State: "creating"})
}

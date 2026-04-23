package agentapi

import (
	"encoding/json"
	"net/http"
	"strings"

	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func (s *Server) VpcByNameHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/vpcs/")
	if name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "resource not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		s.getVpc(w, r, name)
	case http.MethodDelete:
		s.deleteVpc(w, r, name)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) getVpc(w http.ResponseWriter, _ *http.Request, name string) {
	state, err := kv.GetFromDB(s.db, "vpc/"+name+"/state")
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "vpc not found"})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(VPC{Name: name, State: state})
}

func (s *Server) deleteVpc(w http.ResponseWriter, _ *http.Request, name string) {
	cmd := dispatcher.DeleteVPCCommand{Name: name}
	if err := s.dispatcher.Prepare(cmd); err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	s.dispatcher.Dispatch(cmd)
	state, _ := kv.GetFromDB(s.db, "vpc/"+name+"/state")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(VPC{Name: name, State: state})
}

package agentapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"git.g3e.fr/syonad/two/internal/dispatcher"
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
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(VPC{Name: name, State: "created"})
}

func (s *Server) deleteVpc(w http.ResponseWriter, _ *http.Request, name string) {
	s.dispatcher.Dispatch(dispatcher.DeleteVPCCommand{Name: name})
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(VPC{Name: name, State: "deleting"})
}


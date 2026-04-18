package agentapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"git.g3e.fr/syonad/two/internal/dispatcher"
)

func (s *Server) SubnetByNameHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/subnets/")
	if name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "resource not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		s.getSubnet(w, r, name)
	case http.MethodDelete:
		s.deleteSubnet(w, r, name)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) getSubnet(w http.ResponseWriter, r *http.Request, name string) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(Subnet{Name: name, State: "created"})
}

func (s *Server) deleteSubnet(w http.ResponseWriter, r *http.Request, name string) {
	s.dispatcher.Dispatch(dispatcher.DeleteSubnetCommand{Name: name})
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(Subnet{Name: name, State: "deleting"})
}


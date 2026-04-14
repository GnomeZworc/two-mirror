package agentapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) VpcByNameHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/vpcs/")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"name": name})
	case http.MethodDelete:
		s.queue.Submit(func() {
			deleteVpc(name)
		})
		w.WriteHeader(http.StatusAccepted)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func deleteVpc(name string) {}

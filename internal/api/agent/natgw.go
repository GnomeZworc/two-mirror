package agentapi

import (
	"encoding/json"
	"net/http"
	"strings"

	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func (s *Server) NATGatewayByNameHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/nat-gateways/")
	w.Header().Set("Content-Type", "application/json")
	if name == "" {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "resource not found"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getNATGateway(w, r, name)
	case http.MethodDelete:
		s.deleteNATGateway(w, r, name)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) getNATGateway(w http.ResponseWriter, _ *http.Request, name string) {
	prefix := "nat-gateway/" + name + "/"
	state, err := kv.GetFromDB(s.db, prefix+"state")
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "nat gateway not found"})
		return
	}
	vpc, _ := kv.GetFromDB(s.db, prefix+"vpc")
	ip, _ := kv.GetFromDB(s.db, prefix+"ip")
	gateway, _ := kv.GetFromDB(s.db, prefix+"gateway")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(NATGateway{Name: name, State: state, VPC: vpc, IP: ip, Gateway: gateway})
}

func (s *Server) deleteNATGateway(w http.ResponseWriter, _ *http.Request, name string) {
	cmd := dispatcher.DeleteNATGatewayCommand{Name: name}
	if err := s.dispatcher.Prepare(cmd); err != nil {
		if _, dbErr := kv.GetFromDB(s.db, "nat-gateway/"+name+"/state"); dbErr != nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusConflict)
		}
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	s.dispatcher.Dispatch(cmd)
	state, err := kv.GetFromDB(s.db, "nat-gateway/"+name+"/state")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read nat gateway state"})
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(NATGateway{Name: name, State: state})
}

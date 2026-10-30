package agentapi

import (
	"encoding/json"
	"net/http"
	"strings"

	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func (s *Server) FloatingIPByNameHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/floating-ips/")
	w.Header().Set("Content-Type", "application/json")
	if name == "" {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "resource not found"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getFloatingIP(w, r, name)
	case http.MethodDelete:
		s.deleteFloatingIP(w, r, name)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) getFloatingIP(w http.ResponseWriter, _ *http.Request, name string) {
	prefix := "floating-ip/" + name + "/"
	state, err := kv.GetFromDB(s.db, prefix+"state")
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "floating ip not found"})
		return
	}
	vm, _ := kv.GetFromDB(s.db, prefix+"vm")
	iface, _ := kv.GetFromDB(s.db, prefix+"interface")
	ip, _ := kv.GetFromDB(s.db, prefix+"ip")
	gateway, _ := kv.GetFromDB(s.db, prefix+"gateway")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(FloatingIP{Name: name, State: state, VM: vm, Interface: iface, IP: ip, Gateway: gateway})
}

func (s *Server) deleteFloatingIP(w http.ResponseWriter, _ *http.Request, name string) {
	cmd := dispatcher.DeleteFloatingIPCommand{Name: name}
	if err := s.dispatcher.Prepare(cmd); err != nil {
		if _, dbErr := kv.GetFromDB(s.db, "floating-ip/"+name+"/state"); dbErr != nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusConflict)
		}
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	s.dispatcher.Dispatch(cmd)
	state, err := kv.GetFromDB(s.db, "floating-ip/"+name+"/state")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read floating ip state"})
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(FloatingIP{Name: name, State: state})
}

package agentapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
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

func (s *Server) getSubnet(w http.ResponseWriter, _ *http.Request, name string) {
	entries, err := kv.ListByPrefix(s.db, "subnet/"+name+"/")
	if err != nil || len(entries) == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "subnet not found"})
		return
	}
	sub := Subnet{Name: name}
	for key, value := range entries {
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			continue
		}
		switch parts[2] {
		case "state":
			sub.State = value
		case "vpc":
			sub.VPC = value
		case "vxlan_id":
			sub.VxlanID, _ = strconv.Atoi(value)
		case "local_iface":
			sub.LocalIface = value
		case "gateway_ip":
			sub.GatewayIP = value
		case "cidr":
			sub.CIDR = value
		}
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(sub)
}

func (s *Server) deleteSubnet(w http.ResponseWriter, r *http.Request, name string) {
	s.dispatcher.Dispatch(dispatcher.DeleteSubnetCommand{Name: name})
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(Subnet{Name: name, State: "deleting"})
}

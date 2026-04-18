package agentapi

import (
	"encoding/json"
	"net/http"

	"git.g3e.fr/syonad/two/internal/dispatcher"
)

func (s *Server) SubnetsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		s.listSubnets(w, r)
	case http.MethodPost:
		s.postSubnet(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) listSubnets(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode([]Subnet{})
}

func (s *Server) postSubnet(w http.ResponseWriter, r *http.Request) {
	var req SubnetCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request body"})
		return
	}
	if req.Name == "" || req.VPC == "" || req.LocalIP == "" || req.GatewayIP == "" || req.CIDR == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name, vpc, local_ip, gateway_ip and cidr are required"})
		return
	}
	s.dispatcher.Dispatch(dispatcher.CreateSubnetCommand{
		Name:      req.Name,
		VPC:       req.VPC,
		VxlanID:   req.VxlanID,
		LocalIP:   req.LocalIP,
		GatewayIP: req.GatewayIP,
		CIDR:      req.CIDR,
	})
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(Subnet{
		Name:      req.Name,
		State:     "creating",
		VPC:       req.VPC,
		VxlanID:   req.VxlanID,
		LocalIP:   req.LocalIP,
		GatewayIP: req.GatewayIP,
		CIDR:      req.CIDR,
	})
}


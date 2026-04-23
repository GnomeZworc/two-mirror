package agentapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"git.g3e.fr/syonad/two/internal/dispatcher"
	"git.g3e.fr/syonad/two/pkg/db/kv"
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

func (s *Server) listSubnets(w http.ResponseWriter, _ *http.Request) {
	entries, err := kv.ListByPrefix(s.db, "subnet/")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to list subnets"})
		return
	}
	subnets := make(map[string]*Subnet)
	for key, value := range entries {
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			continue
		}
		name := parts[1]
		if _, ok := subnets[name]; !ok {
			subnets[name] = &Subnet{Name: name}
		}
		switch parts[2] {
		case "state":
			subnets[name].State = value
		case "vpc":
			subnets[name].VPC = value
		case "vxlan_id":
			subnets[name].VxlanID, _ = strconv.Atoi(value)
		case "local_iface":
			subnets[name].LocalIface = value
		case "gateway_ip":
			subnets[name].GatewayIP = value
		case "cidr":
			subnets[name].CIDR = value
		}
	}
	result := make([]Subnet, 0, len(subnets))
	for _, sub := range subnets {
		result = append(result, *sub)
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

func (s *Server) postSubnet(w http.ResponseWriter, r *http.Request) {
	var req SubnetCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request body"})
		return
	}
	if req.Name == "" || req.VPC == "" || req.IfaceType == "" || req.GatewayIP == "" || req.CIDR == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name, vpc, iface_type, gateway_ip and cidr are required"})
		return
	}
	s.dispatcher.Dispatch(dispatcher.CreateSubnetCommand{
		Name:      req.Name,
		VPC:       req.VPC,
		VxlanID:   req.VxlanID,
		IfaceType: req.IfaceType,
		GatewayIP: req.GatewayIP,
		CIDR:      req.CIDR,
	})
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(Subnet{
		Name:      req.Name,
		State:     "creating",
		VPC:       req.VPC,
		VxlanID:   req.VxlanID,
		GatewayIP: req.GatewayIP,
		CIDR:      req.CIDR,
	})
}

package agentapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"git.g3e.fr/syonad/two/internal/subnet"
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
		setSubnetField(subnets[name], parts[2], value)
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
	if req.Name == "" || req.VPC == "" || req.CIDR == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name, vpc and cidr are required"})
		return
	}
	if req.Mode != "" && !subnet.ValidMode(req.Mode) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "unknown subnet mode " + strconv.Quote(req.Mode)})
		return
	}
	vxlan := req.Mode == "" || req.Mode == subnet.ModeVxlan
	if !vxlan && req.InterfaceIP == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "interface_ip is required for " + req.Mode + " subnets"})
		return
	}
	if vxlan && req.InterfaceIP != "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "interface_ip is computed for vxlan subnets, do not set it"})
		return
	}
	if req.DefaultRoute && req.Gateway == "" && req.Mode != subnet.ModeBridge {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "default_route without gateway is only allowed for bridge subnets: the host gateway is not on the subnet"})
		return
	}
	cmd := dispatcher.CreateSubnetCommand{
		Name:         req.Name,
		VPC:          req.VPC,
		Mode:         req.Mode,
		VxlanID:      req.VxlanID,
		IfaceType:    req.IfaceType,
		InterfaceIP:  req.InterfaceIP,
		CIDR:         req.CIDR,
		DefaultRoute: req.DefaultRoute,
		Gateway:      req.Gateway,
	}
	if err := s.dispatcher.Prepare(cmd); err != nil {
		if _, dbErr := kv.GetFromDB(s.db, "subnet/"+req.Name+"/state"); dbErr == nil {
			w.WriteHeader(http.StatusConflict)
		} else {
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	s.dispatcher.Dispatch(cmd)
	entries, err := kv.ListByPrefix(s.db, "subnet/"+req.Name+"/")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read subnet state"})
		return
	}
	sub := subnetFromEntries(req.Name, entries)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(sub)
}

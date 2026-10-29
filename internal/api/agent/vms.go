package agentapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"git.g3e.fr/syonad/two/internal/metadata"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

var diskDev = regexp.MustCompile(`^(vda|xvd[a-z])$`)

func (s *Server) VmsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		s.listVMs(w, r)
	case http.MethodPost:
		s.startVM(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "method not allowed"})
	}
}

func (s *Server) listVMs(w http.ResponseWriter, _ *http.Request) {
	entries, err := kv.ListByPrefix(s.db, "vm/")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to list vms"})
		return
	}

	names := map[string]struct{}{}
	for key := range entries {
		parts := strings.Split(key, "/")
		if len(parts) >= 2 {
			names[parts[1]] = struct{}{}
		}
	}

	result := make([]VM, 0, len(names))
	for name := range names {
		vm, err := vmFromDB(name, entries)
		if err != nil {
			continue
		}
		result = append(result, vm)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

func (s *Server) startVM(w http.ResponseWriter, r *http.Request) {
	var req VMCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request body"})
		return
	}
	if req.Name == "" || len(req.Interfaces) == 0 || len(req.Storage) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name, interfaces and storage are required"})
		return
	}

	nics := make([]dispatcher.VMNIC, len(req.Interfaces))
	primaries := 0
	names := make(map[string]int, len(req.Interfaces))
	for i, iface := range req.Interfaces {
		if iface.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("interface %d: name is required", i)})
			return
		}
		if other, ok := names[iface.Name]; ok {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("interface %d: name %q is already used by interface %d", i, iface.Name, other)})
			return
		}
		names[iface.Name] = i
		if iface.MAC == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("interface %d: mac is required", i)})
			return
		}
		nics[i] = dispatcher.VMNIC{Name: iface.Name, Subnet: iface.Subnet, IP: iface.IP, MAC: iface.MAC, Primary: iface.Primary, DHCPServer: iface.DHCPServer}
		if iface.Primary {
			primaries++
		}
	}
	if primaries != 1 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "exactly one interface must be primary"})
		return
	}

	disks := make([]dispatcher.VMDisk, len(req.Storage))
	for i, s := range req.Storage {
		if !diskDev.MatchString(s.Dev) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("storage %d: dev %q must be vda (root disk) or xvd[a-z]", i, s.Dev)})
			return
		}
		disks[i] = dispatcher.VMDisk{Path: s.Path, Dev: s.Dev}
	}

	documents, err := decodeDocuments(req.Metadata)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}

	cmd := dispatcher.StartVMCommand{
		Name:      req.Name,
		NICs:      nics,
		Disks:     disks,
		Memory:    req.Memory,
		CPUs:      req.CPUs,
		UEFI:      req.UEFI,
		Password:  req.Metadata.Password,
		SSHKey:    req.Metadata.SSHKey,
		Documents: documents,
	}

	if err := s.dispatcher.Prepare(cmd); err != nil {
		if _, dbErr := kv.GetFromDB(s.db, "vm/"+req.Name+"/state"); dbErr == nil {
			w.WriteHeader(http.StatusConflict)
		} else {
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	s.dispatcher.Dispatch(cmd)

	entries, _ := kv.ListByPrefix(s.db, "vm/"+req.Name+"/")
	vm, err := vmFromDB(req.Name, entries)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read vm state"})
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(vm)
}

func decodeDocuments(m VMMetadata) (map[string]string, error) {
	if m.UserData == "" {
		return nil, nil
	}
	content, err := base64.StdEncoding.DecodeString(m.UserData)
	if err != nil {
		return nil, fmt.Errorf("metadata.user_data is not valid base64: %w", err)
	}
	return map[string]string{metadata.DocUserData: string(content)}, nil
}

package agentapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func (s *Server) VmByNameHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/vms/")
	if name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "resource not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		s.getVM(w, r, name)
	case http.MethodDelete:
		s.stopVM(w, r, name)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "method not allowed"})
	}
}

func (s *Server) getVM(w http.ResponseWriter, _ *http.Request, name string) {
	entries, err := kv.ListByPrefix(s.db, "vm/"+name+"/")
	if err != nil || len(entries) == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "vm not found"})
		return
	}
	vm, err := vmFromDB(name, entries)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read vm"})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(vm)
}

func (s *Server) stopVM(w http.ResponseWriter, _ *http.Request, name string) {
	cmd := dispatcher.StopVMCommand{Name: name}
	if err := s.dispatcher.Prepare(cmd); err != nil {
		if _, dbErr := kv.GetFromDB(s.db, "vm/"+name+"/state"); dbErr != nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusConflict)
		}
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	s.dispatcher.Dispatch(cmd)

	entries, _ := kv.ListByPrefix(s.db, "vm/"+name+"/")
	vm, err := vmFromDB(name, entries)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read vm state"})
		return
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(vm)
}

func vmFromDB(name string, entries map[string]string) (VM, error) {
	prefix := "vm/" + name + "/"
	vm := VM{Name: name}

	vm.State = entries[prefix+"state"]
	vm.MetadataPort = entries[prefix+"metadata_port"]
	vm.Memory, _ = strconv.Atoi(entries[prefix+"memory"])
	vm.CPUs, _ = strconv.Atoi(entries[prefix+"cpus"])

	subnet := entries[prefix+"subnet"]
	ip := entries[prefix+"ip"]
	if subnet != "" || ip != "" {
		vm.Interfaces = []VMInterface{{Subnet: subnet, IP: ip, Primary: true}}
	}

	if path := entries[prefix+"volume_path"]; path != "" {
		vm.Storage = []VMStorage{{Path: path}}
	}

	return vm, nil
}

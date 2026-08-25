package agentapi

import (
	"encoding/json"
	"net/http"
	"sort"
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

// interfacesFromDB reconstruit les interfaces depuis vm/<name>/nic/<index>/…,
// triées par index — celui-ci détermine le slot PCI, donc le nom de l'interface
// dans le guest.
func interfacesFromDB(prefix string, entries map[string]string) []VMInterface {
	nicPrefix := prefix + "nic/"
	byIndex := make(map[int]*VMInterface)

	for key, value := range entries {
		rest := strings.TrimPrefix(key, nicPrefix)
		if rest == key {
			continue
		}
		parts := strings.Split(rest, "/")
		if len(parts) != 2 {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		if byIndex[idx] == nil {
			byIndex[idx] = &VMInterface{}
		}
		switch parts[1] {
		case "subnet":
			byIndex[idx].Subnet = value
		case "ip":
			byIndex[idx].IP = value
		case "primary":
			byIndex[idx].Primary = value == "true"
		}
	}

	indexes := make([]int, 0, len(byIndex))
	for idx := range byIndex {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)

	ifaces := make([]VMInterface, 0, len(indexes))
	for _, idx := range indexes {
		ifaces = append(ifaces, *byIndex[idx])
	}
	return ifaces
}

func vmFromDB(name string, entries map[string]string) (VM, error) {
	prefix := "vm/" + name + "/"
	vm := VM{Name: name}

	vm.State = entries[prefix+"state"]
	vm.MetadataPort = entries[prefix+"metadata_port"]
	vm.Memory, _ = strconv.Atoi(entries[prefix+"memory"])
	vm.CPUs, _ = strconv.Atoi(entries[prefix+"cpus"])
	vm.UEFI = entries[prefix+"uefi"] == "true"

	vm.Interfaces = interfacesFromDB(prefix, entries)

	diskPrefix := prefix + "disk/"
	for key, path := range entries {
		if dev := strings.TrimPrefix(key, diskPrefix); dev != key {
			vm.Storage = append(vm.Storage, VMStorage{Path: path, Dev: dev})
		}
	}

	return vm, nil
}

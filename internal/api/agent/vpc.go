package agentapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"git.g3e.fr/syonad/two/internal/vpc"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func (s *Server) VpcByNameHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/vpcs/")
	if name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "resource not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		s.getVpc(w, r, name)
	case http.MethodDelete:
		s.deleteVpc(w, r, name)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) getVpc(w http.ResponseWriter, _ *http.Request, name string) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(VPC{Name: name, State: "created"})
}

func (s *Server) deleteVpc(w http.ResponseWriter, _ *http.Request, name string) {
	s.queue.Submit(func() {
		kv.AddInDB(s.db, "vpc/"+name+"/state", "deleting")
		if err := vpc.DeleteVPC(s.db, name); err != nil {
			fmt.Println(err)
		}
		if state, err := kv.GetFromDB(s.db, "vpc/"+name+"/state"); err != nil {
			fmt.Println(err)
			os.Exit(1)
		} else if state == "deleted" {
			kv.DeleteInDB(s.db, "vpc/"+name)
		}
	})
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(VPC{Name: name, State: "deleting"})
}

func destroyVpc(name string) {}

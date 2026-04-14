package agentapi

import (
	"log"
	"net/http"

	"git.g3e.fr/syonad/two/pkg/worker"
)

type Server struct {
	queue *worker.Queue
}

func New(queue *worker.Queue) *Server {
	return &Server{queue: queue}
}

func (s *Server) Start(address string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/vpcs", s.VpcsHandler)
	mux.HandleFunc("/vpcs/", s.VpcByNameHandler)
	mux.HandleFunc("/subnets", s.SubnetsHandler)
	mux.HandleFunc("/subnets/", s.SubnetByNameHandler)
	log.Printf("API server listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

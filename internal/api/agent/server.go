package agentapi

import (
	"log"
	"net/http"

	"git.g3e.fr/syonad/two/internal/dispatcher"
	"github.com/dgraph-io/badger/v4"
)

type Server struct {
	dispatcher *dispatcher.Dispatcher
	db         *badger.DB
}

func New(d *dispatcher.Dispatcher, db *badger.DB) *Server {
	return &Server{dispatcher: d, db: db}
}

func (s *Server) Start(address string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/vpcs", s.VpcsHandler)
	mux.HandleFunc("/vpcs/", s.VpcByNameHandler)
	mux.HandleFunc("/subnets", s.SubnetsHandler)
	mux.HandleFunc("/subnets/", s.SubnetByNameHandler)
	log.Printf("API server listening on %s", address)
	log.Fatal(http.ListenAndServe(address, logMiddleware(mux)))
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

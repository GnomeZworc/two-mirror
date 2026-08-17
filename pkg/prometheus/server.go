package promserver

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server exposes the Prometheus metrics endpoint.
type Server struct {
	srv *http.Server
}

// New builds the metrics server for the given address and registry.
func New(address string, registry *prometheus.Registry) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	return &Server{srv: &http.Server{Addr: address, Handler: mux}}
}

// Start blocks until the server stops. A failure is logged, never fatal: an
// unavailable metrics endpoint must not bring the whole agent down.
func (s *Server) Start() {
	log.Printf("Prometheus server listening on %s", s.srv.Addr)
	if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("Prometheus server stopped: %v", err)
	}
}

// Shutdown stops the server, waiting for in-flight requests until ctx expires.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

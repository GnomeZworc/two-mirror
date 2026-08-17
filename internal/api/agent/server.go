package agentapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"github.com/dgraph-io/badger/v4"
)

type Server struct {
	dispatcher *dispatcher.Dispatcher
	db         *badger.DB
	logger     *slog.Logger
	srv        *http.Server
}

func New(d *dispatcher.Dispatcher, db *badger.DB, logger *slog.Logger, address string) *Server {
	s := &Server{dispatcher: d, db: db, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("/vpcs", s.VpcsHandler)
	mux.HandleFunc("/vpcs/", s.VpcByNameHandler)
	mux.HandleFunc("/subnets", s.SubnetsHandler)
	mux.HandleFunc("/subnets/", s.SubnetByNameHandler)
	mux.HandleFunc("/vms", s.VmsHandler)
	mux.HandleFunc("/vms/", s.VmByNameHandler)
	s.srv = &http.Server{Addr: address, Handler: s.logMiddleware(mux)}
	return s
}

func (s *Server) Start() {
	s.logger.Info("API server listening", "address", s.srv.Addr)
	if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("API server stopped", "error", err)
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b [4]byte
		rand.Read(b[:])
		reqID := hex.EncodeToString(b[:])

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(sw, r)

		s.logger.Info("request",
			"request_id", reqID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

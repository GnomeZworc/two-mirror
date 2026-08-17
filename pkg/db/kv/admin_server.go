package kv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"

	"github.com/dgraph-io/badger/v4"
)

type AdminServer struct {
	db     *badger.DB
	logger *slog.Logger
	srv    *http.Server
}

func NewAdminServer(db *badger.DB, logger *slog.Logger, address string) *AdminServer {
	s := &AdminServer{db: db, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("/db", s.dbHandler)
	s.srv = &http.Server{Addr: address, Handler: mux}
	return s
}

func (s *AdminServer) Start() {
	s.logger.Info("admin server listening", "address", s.srv.Addr)
	if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("admin server stopped", "error", err)
	}
}

func (s *AdminServer) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *AdminServer) dbHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entries, err := ListByPrefix(s.db, r.URL.Query().Get("prefix"))
	if err != nil {
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, k := range keys {
		fmt.Fprintf(w, "%s=%s\n", k, entries[k])
	}
}

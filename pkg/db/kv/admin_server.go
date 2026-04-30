package kv

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"

	"github.com/dgraph-io/badger/v4"
)

type AdminServer struct {
	db     *badger.DB
	logger *slog.Logger
}

func NewAdminServer(db *badger.DB, logger *slog.Logger) *AdminServer {
	return &AdminServer{db: db, logger: logger}
}

func (s *AdminServer) Start(address string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/db", s.dbHandler)
	s.logger.Info("admin server listening", "address", address)
	if err := http.ListenAndServe(address, mux); err != nil {
		s.logger.Error("admin server stopped", "error", err)
	}
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

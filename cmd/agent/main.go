package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	agentapi "git.g3e.fr/syonad/two/internal/api/agent"
	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	"git.g3e.fr/syonad/two/internal/migration"
	agentmetrics "git.g3e.fr/syonad/two/internal/prometheus/agent"
	"git.g3e.fr/syonad/two/internal/watchdog"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"git.g3e.fr/syonad/two/pkg/logger"
	promserver "git.g3e.fr/syonad/two/pkg/prometheus"
	"git.g3e.fr/syonad/two/pkg/worker"
	"github.com/prometheus/client_golang/prometheus"
)

const shutdownTimeout = 20 * time.Second

func main() {
	confFile := flag.String("config", "/etc/two/agent.yml", "config file path")
	flag.Parse()

	cfg, err := configuration.LoadConfig(*confFile)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return
	}

	log := logger.New(cfg.Logger.Level, cfg.Logger.Debug)

	db := kv.InitDB(kv.Config{Path: cfg.Database.Path}, false)
	closeDB := true
	defer func() {
		if closeDB {
			db.Close()
		}
	}()

	// Avant tout démarrage de service : la DB peut porter l'ancien vocabulaire
	// d'états, et des ressources transitoires orphelines d'un arrêt précédent.
	if err := migration.MigrateStates(db, log.With(slog.String("component", "migration"))); err != nil {
		log.Error("failed to migrate states", "error", err)
		return
	}

	q := worker.New(cfg.Worker.BufferSize)
	q.Start(cfg.Worker.Count)

	registry := prometheus.NewRegistry()
	registry.MustRegister(agentmetrics.NewAgentCollector(db))

	apiAddr := fmt.Sprintf("%s:%d", cfg.Api.Address, cfg.Api.Port)
	promAddr := fmt.Sprintf("%s:%d", cfg.Prometheus.Address, cfg.Prometheus.Port)

	log.Info("starting agent",
		"api", apiAddr,
		"prometheus", promAddr,
		"workers", cfg.Worker.Count,
		"log_level", cfg.Logger.Level,
		"debug", cfg.Logger.Debug,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d := dispatcher.New(q, db, cfg, log.With(slog.String("component", "dispatcher")))

	apiSrv := agentapi.New(d, db, log.With(slog.String("component", "api")), apiAddr)
	go apiSrv.Start()

	promSrv := promserver.New(promAddr, registry)
	go promSrv.Start()

	var adminSrv *kv.AdminServer
	if cfg.Admin.Enabled {
		adminAddr := fmt.Sprintf("%s:%d", cfg.Admin.Address, cfg.Admin.Port)
		adminSrv = kv.NewAdminServer(db, log.With(slog.String("component", "admin")), adminAddr)
		go adminSrv.Start()
	}

	if cfg.Watchdog.Enabled {
		wlog := log.With(slog.String("component", "watchdog"))
		go watchdog.New(db, cfg, notify.NewStderr(wlog), wlog,
			time.Duration(cfg.Watchdog.IntervalSeconds)*time.Second,
		).Run(ctx)
	}

	<-ctx.Done()
	stop()

	servers := map[string]httpShutdowner{"api": apiSrv, "prometheus": promSrv}
	if adminSrv != nil {
		servers["admin"] = adminSrv
	}
	closeDB = shutdown(log, q, servers, shutdownTimeout)
}

type httpShutdowner interface {
	Shutdown(context.Context) error
}

func shutdown(log *slog.Logger, q *worker.Queue, servers map[string]httpShutdowner, timeout time.Duration) bool {
	log.Info("shutting down", "timeout", timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for name, srv := range servers {
		if err := srv.Shutdown(ctx); err != nil {
			log.Error("http server shutdown", "server", name, "error", err)
		}
	}

	drained := make(chan struct{})
	go func() {
		q.Stop()
		close(drained)
	}()

	select {
	case <-drained:
		log.Info("workers drained")
		return true
	case <-ctx.Done():
		log.Error("workers still running after timeout, leaving database untouched")
		return false
	}
}

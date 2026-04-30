package main

import (
	"flag"
	"fmt"
	"log/slog"

	agentapi "git.g3e.fr/syonad/two/internal/api/agent"
	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	dispatcher "git.g3e.fr/syonad/two/internal/dispatcher/agent"
	agentmetrics "git.g3e.fr/syonad/two/internal/prometheus/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"git.g3e.fr/syonad/two/pkg/logger"
	promserver "git.g3e.fr/syonad/two/pkg/prometheus"
	"git.g3e.fr/syonad/two/pkg/worker"
	"github.com/prometheus/client_golang/prometheus"
)

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
	defer db.Close()

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

	d := dispatcher.New(q, db, cfg, log.With(slog.String("component", "dispatcher")))
	go agentapi.New(d, db, log.With(slog.String("component", "api"))).Start(apiAddr)
	go promserver.Start(promAddr, registry)
	if cfg.Admin.Enabled {
		adminAddr := fmt.Sprintf("%s:%d", cfg.Admin.Address, cfg.Admin.Port)
		go kv.NewAdminServer(db, log.With(slog.String("component", "admin"))).Start(adminAddr)
	}

	select {}
}

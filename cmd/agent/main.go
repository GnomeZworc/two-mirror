package main

import (
	"flag"
	"fmt"
	"log"

	agentapi "git.g3e.fr/syonad/two/internal/api/agent"
	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/dispatcher"
	agentmetrics "git.g3e.fr/syonad/two/internal/prometheus/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	promserver "git.g3e.fr/syonad/two/pkg/prometheus"
	"git.g3e.fr/syonad/two/pkg/worker"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	confFile := flag.String("config", "/etc/two/agent.yml", "config file path")
	flag.Parse()

	cfg, err := configuration.LoadConfig(*confFile)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	db := kv.InitDB(kv.Config{Path: cfg.Database.Path}, false)
	defer db.Close()

	q := worker.New(cfg.Worker.BufferSize)
	q.Start(cfg.Worker.Count)

	registry := prometheus.NewRegistry()
	registry.MustRegister(agentmetrics.NewAgentCollector(db))

	apiAddr := fmt.Sprintf("%s:%d", cfg.Api.Address, cfg.Api.Port)
	promAddr := fmt.Sprintf("%s:%d", cfg.Prometheus.Address, cfg.Prometheus.Port)

	d := dispatcher.New(q, db)
	go agentapi.New(d, db).Start(apiAddr)
	go promserver.Start(promAddr, registry)

	select {}
}

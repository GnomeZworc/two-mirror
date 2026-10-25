package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	hostsvcapi "git.g3e.fr/syonad/two/internal/api/hostsvc"
	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/hostsvc"
	"git.g3e.fr/syonad/two/pkg/logger"
)

var (
	confFile = flag.String("conf", "/etc/two/agent.yml", "configuration file")
	runDir   = flag.String("run-dir", hostsvcapi.DefaultRunDir, "directory of the control socket and the state file")
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "two-hostsvc: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := configuration.LoadConfig(*confFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.Logger.Level, cfg.Logger.Debug)

	svc, err := hostsvc.New(hostsvc.Options{
		DNS:    cfg.HostSvc.DNS(),
		NTP:    cfg.HostSvc.NTP(),
		Rate:   cfg.HostSvc.Rate(),
		Logger: log,
	}, hostsvcapi.StatePath(*runDir))
	if err != nil {
		return err
	}
	if err := svc.Restore(); err != nil {
		log.Error("some vpcs could not be restored", "error", err)
	}

	control, err := hostsvcapi.Listen(svc, hostsvcapi.SocketPath(*runDir), log)
	if err != nil {
		return fmt.Errorf("listen on the control socket: %w", err)
	}
	defer control.Close()

	log.Info("two-hostsvc started", "socket", control.Addr(), "vpcs", svc.List())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := control.Serve(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Error("control socket stopped", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("two-hostsvc stopping")
	if err := control.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Error("close the control socket", "error", err)
	}
	return svc.Shutdown()
}

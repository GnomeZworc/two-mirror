package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	dhcpapi "git.g3e.fr/syonad/two/internal/api/dhcp"
	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/dhcpd"
	"git.g3e.fr/syonad/two/pkg/logger"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
)

var (
	confFile   = flag.String("conf", "/etc/two/agent.yml", "configuration file")
	iface      = flag.String("interface", "", "bridge to serve, already present in the current network namespace")
	statePath  = flag.String("state", "", "state file owned by this process")
	socketPath = flag.String("socket", "", "control socket the agent talks to")
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dhcp: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	for name, value := range map[string]string{
		"-interface": *iface,
		"-state":     *statePath,
		"-socket":    *socketPath,
	} {
		if value == "" {
			return fmt.Errorf("%s is required", name)
		}
	}

	cfg, err := configuration.LoadConfig(*confFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.Logger.Level, cfg.Logger.Debug).With("bridge", *iface)

	store := dhcpd.NewStore(*statePath)
	if err := store.Load(); err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	control, err := dhcpapi.Listen(store, *socketPath, log)
	if err != nil {
		return fmt.Errorf("listen on the control socket: %w", err)
	}
	defer control.Close()

	go func() {
		if err := control.Serve(); err != nil {
			log.Error("control socket stopped", "error", err)
		}
	}()

	conn, err := server4.NewIPv4UDPConn(*iface, &net.UDPAddr{Port: dhcpv4.ServerPort})
	if err != nil {
		return fmt.Errorf("bind udp/%d on %s: %w", dhcpv4.ServerPort, *iface, err)
	}
	defer conn.Close()

	log.Info("dhcp server started", "state", store.Path(), "socket", control.Addr())

	return store.Serve(conn, log)
}

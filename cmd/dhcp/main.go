package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"git.g3e.fr/syonad/two/internal/dhcp"
	"git.g3e.fr/syonad/two/pkg/systemd"
)

func main() {
	subnet := flag.String("subnet", "", "Subnet CIDR (e.g. 10.10.10.0/24)")
	name := flag.String("name", "", "Config name (e.g. vpc1_br-00002)")
	gateway := flag.String("gateway", "", "Gateway IP (e.g. 10.10.10.1)")
	confDir := flag.String("confdir", "/etc/dnsmasq.d", "dnsmasq config directory")
	flag.Parse()

	if *subnet == "" || *name == "" || *gateway == "" {
		flag.Usage()
		os.Exit(1)
	}

	_, network, err := net.ParseCIDR(*subnet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid subnet: %v\n", err)
		os.Exit(1)
	}

	gw := net.ParseIP(*gateway)
	if gw == nil {
		fmt.Fprintf(os.Stderr, "invalid gateway IP: %q\n", *gateway)
		os.Exit(1)
	}

	conf := dhcp.Config{
		Network: network,
		Gateway: gw,
		Name:    *name,
		ConfDir: *confDir,
	}

	confPath, err := dhcp.GenerateConfig(conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error generating config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("dnsmasq config written to %s\n", confPath)

	svc, err := systemd.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error connecting to systemd: %v\n", err)
		os.Exit(1)
	}
	defer svc.Close()

	unit := "dnsmasq@" + *name + ".service"
	if err := svc.Start(unit); err != nil {
		fmt.Fprintf(os.Stderr, "error starting %s: %v\n", unit, err)
		os.Exit(1)
	}
	fmt.Printf("started %s\n", unit)
}

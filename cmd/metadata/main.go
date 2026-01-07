package main

import (
	"flag"

	"git.g3e.fr/syonad/two/internal/metadata"
)

var (
	iface      = flag.String("interface", "0.0.0.0", "Interface IP à écouter")
	port       = flag.Int("port", 0, "Port à utiliser")
	netns_name = flag.String("netns", "", "Network namespace à utiliser")
	conf_file  = flag.String("conf", "/etc/two/agent.yml", "configuration file")
	vm_name    = flag.String("vm", "", "Name of the vm")
)

func main() {
	flag.Parse()

	metadata.StartServer(metadata.ServerConfig{
		Netns:    *netns_name,
		Iface:    *iface,
		Port:     *port,
		ConfFile: *conf_file,
		VmName:   *vm_name,
	})
}

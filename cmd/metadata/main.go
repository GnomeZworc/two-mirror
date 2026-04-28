package main

import (
	"flag"
	"fmt"
	"os"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/metadata"
)

var (
	confFile = flag.String("conf", "/etc/two/agent.yml", "configuration file")
	vm_name  = flag.String("vm", "", "Name of the vm")
)

func main() {
	flag.Parse()

	cfg, err := configuration.LoadConfig(*confFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	metadata.StartServer(metadata.ServerConfig{
		VmName: *vm_name,
		RunDir: cfg.Metadata.RunDir,
	})
}

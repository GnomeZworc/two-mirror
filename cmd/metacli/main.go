package main

import (
	"flag"
	"fmt"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/metadata"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func main() {
	conf_file := flag.String("conf", "/etc/two/agent.yml", "configuration file")
	vm_name := flag.String("vm_name", "", "Nom de la vm")
	vpc := flag.String("vpc_name", "", "vpc name")
	bind_ip := flag.String("ip", "", "bind ip")
	bind_port := flag.String("port", "", "bind port")
	ssh_key := flag.String("key", "", "Clef ssh")
	password := flag.String("pass", "", "password user")
	start := flag.Bool("start", false, "start metadata server")
	stop := flag.Bool("stop", false, "stop metadata server")
	dryrun := flag.Bool("dryrun", false, "launch in dry node")

	flag.Parse()

	conf, err := configuration.LoadConfig(*conf_file)
	if err != nil {
		fmt.Println(err)
		return
	}

	db := kv.InitDB(kv.Config{
		Path: conf.Database.Path,
	}, false)
	defer db.Close()

	if *start {
		if err := metadata.StartMetadata(metadata.NoCloudConfig{
			VpcName:  *vpc,
			Name:     *vm_name,
			BindIP:   *bind_ip,
			BindPort: *bind_port,
			Password: *password,
			SSHKEY:   *ssh_key,
		}, db, *dryrun); err != nil {
			fmt.Println(err)
		}
	} else if *stop {
		if err := metadata.StopMetadata(*vm_name, db, *dryrun); err != nil {
			fmt.Println(err)
		}
	}
}

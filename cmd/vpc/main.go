package main

import (
	"flag"
	"fmt"
	"os"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/vpc"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

var (
	netns     = flag.String("netns", "", "Network namespace à faire")
	name      = flag.String("name", "", "interface name")
	action    = flag.String("action", "", "Action a faire")
	conf_file = flag.String("conf", "/etc/two/agent.yml", "configuration file")
)

var DB *badger.DB

func main() {
	flag.Parse()

	conf, err := configuration.LoadConfig(*conf_file)
	if err != nil {
		fmt.Println(err)
		return
	}

	DB = kv.InitDB(kv.Config{
		Path: conf.Database.Path,
	}, false)
	defer DB.Close()

	switch *action {
	case "create":
		kv.AddInDB(DB, "vpc/"+*name+"/state", "creating")
		if err := vpc.CreateVPC(DB, *name); err != nil {
			fmt.Println(err)
		}
	case "delete":
		if err := vpc.DeleteVPC(*name); err != nil {
			fmt.Println(err)
		}
	default:
		fmt.Printf("Available commande:\n - create\n - delete\n - check\n")
		os.Exit(1)
	}
}

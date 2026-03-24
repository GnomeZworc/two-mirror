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
		kv.AddInDB(DB, "vpc/"+*name+"/state", "deleting")
		if err := vpc.DeleteVPC(DB, *name); err != nil {
			fmt.Println(err)
		}
		if state, err := kv.GetFromDB(DB, "vpc/"+*name+"/state"); err != nil {
			fmt.Println(err)
			os.Exit(1)
		} else if state == "deleted" {
			kv.DeleteInDB(DB, "vpc/"+*name)
		}
	case "check":
		if state, err := kv.GetFromDB(DB, "vpc/"+*name+"/state"); err != nil {
			os.Exit(1)
		} else if state != "created" {
			os.Exit(1)
		}
	default:
		fmt.Printf("Available commande:\n - create\n - delete\n - check\n")
		os.Exit(1)
	}
	os.Exit(0)
}

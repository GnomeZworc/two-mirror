package main

import (
	"flag"
	"fmt"
	"os"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/subnet"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

var (
	name       = flag.String("name", "", "Subnet name (ex: sn-00001)")
	vpcName    = flag.String("vpc", "", "VPC name")
	vxlanID    = flag.String("vxlan-id", "", "VXLAN ID")
	localIP    = flag.String("local-ip", "", "Local VTEP IP")
	gatewayIP  = flag.String("gateway-ip", "", "Gateway IP")
	cidr       = flag.String("cidr", "", "Subnet CIDR (ex: 10.10.10.0/24)")
	action     = flag.String("action", "", "Action à effectuer")
	conf_file  = flag.String("conf", "/etc/two/agent.yml", "Configuration file")
)

var DB *badger.DB

func main() {
	flag.Parse()

	conf, err := configuration.LoadConfig(*conf_file)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	DB = kv.InitDB(kv.Config{
		Path: conf.Database.Path,
	}, false)
	defer DB.Close()

	switch *action {
	case "create":
		if *name == "" || *vpcName == "" || *vxlanID == "" || *localIP == "" || *gatewayIP == "" || *cidr == "" {
			fmt.Println("create requires: -name -vpc -vxlan-id -local-ip -gateway-ip -cidr")
			os.Exit(1)
		}
		kv.AddInDB(DB, "subnet/"+*name+"/state", "creating")
		kv.AddInDB(DB, "subnet/"+*name+"/vpc", *vpcName)
		kv.AddInDB(DB, "subnet/"+*name+"/vxlan_id", *vxlanID)
		kv.AddInDB(DB, "subnet/"+*name+"/local_ip", *localIP)
		kv.AddInDB(DB, "subnet/"+*name+"/gateway_ip", *gatewayIP)
		kv.AddInDB(DB, "subnet/"+*name+"/cidr", *cidr)
		if err := subnet.CreateSubnet(DB, *name); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

	case "delete":
		if *name == "" {
			fmt.Println("delete requires: -name")
			os.Exit(1)
		}
		kv.AddInDB(DB, "subnet/"+*name+"/state", "deleting")
		if err := subnet.DeleteSubnet(DB, *name); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if state, err := kv.GetFromDB(DB, "subnet/"+*name+"/state"); err != nil {
			fmt.Println(err)
			os.Exit(1)
		} else if state == "deleted" {
			kv.DeleteInDB(DB, "subnet/"+*name)
		}

	case "check":
		if *name == "" {
			fmt.Println("check requires: -name")
			os.Exit(1)
		}
		if state, err := kv.GetFromDB(DB, "subnet/"+*name+"/state"); err != nil {
			os.Exit(1)
		} else if state != "created" {
			os.Exit(1)
		}

	default:
		fmt.Printf("Available commands:\n - create\n - delete\n - check\n")
		os.Exit(1)
	}

	os.Exit(0)
}

package main

import (
	"flag"
	"fmt"
	"os"

	"git.g3e.fr/syonad/two/internal/netif"
)

var (
	netns  = flag.String("netns", "", "Network namespace à faire")
	name   = flag.String("name", "", "interface name")
	action = flag.String("action", "", "Action a faire")
)

func main() {
	flag.Parse()

	switch *action {
	case "create":
		err := netif.CreateVethToNetns("veth"+*name+"ext", "veth"+*name+"int", "/var/run/netns/"+*netns, 9000)
		if err != nil {
			fmt.Println(err)
		}
	default:
		fmt.Printf("Available commande:\n - create\n - delete\n - check\n")
		os.Exit(1)
	}
}

package main

import (
	"flag"
	"fmt"
	"os"

	"git.g3e.fr/syonad/two/internal/netns"
)

var (
	netns_name = flag.String("netns", "", "Network namespace à faire")
	action     = flag.String("action", "", "Action a faire")
)

func main() {
	flag.Parse()

	switch *action {
	case "create":
		err := netns.Create(*netns_name)
		if err != nil {
			fmt.Println(err)
		}
	case "delete":
		err := netns.Delete(*netns_name)
		if err != nil {
			fmt.Println(err)
		}
	case "check":
		if netns.Exist(*netns_name) {
			fmt.Printf("netns %s exist\n", *netns_name)
		} else {
			fmt.Printf("netns %s do not exist\n", *netns_name)
		}
	default:
		fmt.Printf("Available commande:\n - create\n - delete\n - check\n")
		os.Exit(1)
	}
}

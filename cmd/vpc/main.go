package main

import (
	"flag"
	"fmt"
	"os"

	"git.g3e.fr/syonad/two/internal/vpc"
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
		if err := vpc.CreateVPC(*name); err != nil {
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

package main

import (
	"fmt"
	"os"
)

var (
	bin_name = os.Args[0]
)

func main() {

	fmt.Printf("%s: Start process\n", bin_name)

	os.Exit(0)
}

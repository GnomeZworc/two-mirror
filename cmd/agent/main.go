package main

import (
	"fmt"
	"os"
)

func main() {
	bin_name := os.Args[0]

	fmt.Printf("%s: Start process\n", bin_name)

	os.Exit(5)
}

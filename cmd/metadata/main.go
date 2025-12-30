package main

import (
	"flag"
	"log"

	"git.g3e.fr/syonad/two/internal/metadata"
)

var (
	iface      = flag.String("interface", "0.0.0.0", "Interface IP à écouter")
	port       = flag.Int("port", 8080, "Port à utiliser")
	file       = flag.String("file", "", "Fichier JSON contenant les données NoCloud")
	netns_name = flag.String("netns", "", "Network namespace à utiliser")
)

func main() {
	flag.Parse()

	if *file == "" {
		log.Fatal("Vous devez spécifier un fichier via --file")
	}

	metadata.StartServer(metadata.Config{
		Netns: *netns_name,
		File:  *file,
		Iface: *iface,
		Port:  *port,
	})
}

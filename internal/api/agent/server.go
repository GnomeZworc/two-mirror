package agentapi

import (
	"log"
	"net/http"
)

func Start(address string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/vpcs", VpcsHandler)
	mux.HandleFunc("/vpcs/", VpcByNameHandler)
	mux.HandleFunc("/subnets", SubnetsHandler)
	mux.HandleFunc("/subnets/", SubnetByNameHandler)
	log.Printf("API server listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

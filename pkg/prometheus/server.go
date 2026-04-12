package promserver

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Start launches the Prometheus metrics HTTP server on the given address.
// The provided registry is used to expose metrics at /metrics.
func Start(address string, registry *prometheus.Registry) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	log.Printf("Prometheus server listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

package agentmetrics

import (
	"strings"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
	"github.com/prometheus/client_golang/prometheus"
)

var allStates = []string{"creating", "created", "deleting", "deleted"}

// AgentCollector implements prometheus.Collector and exposes agent metrics
// by querying the BadgerDB on each scrape.
type AgentCollector struct {
	db           *badger.DB
	vpcsTotal    *prometheus.Desc
	subnetsTotal *prometheus.Desc
}

func NewAgentCollector(db *badger.DB) *AgentCollector {
	return &AgentCollector{
		db: db,
		vpcsTotal: prometheus.NewDesc(
			"syonad_vpcs_total",
			"Number of VPCs by state.",
			[]string{"state"}, nil,
		),
		subnetsTotal: prometheus.NewDesc(
			"syonad_subnets_total",
			"Number of subnets by state.",
			[]string{"state"}, nil,
		),
	}
}

func (c *AgentCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.vpcsTotal
	ch <- c.subnetsTotal
}

func (c *AgentCollector) Collect(ch chan<- prometheus.Metric) {
	c.collectStates(ch, "vpc/", c.vpcsTotal)
	c.collectStates(ch, "subnet/", c.subnetsTotal)
}

// collectStates counts resources under the given DB prefix by their state value
// and emits one gauge per state label.
func (c *AgentCollector) collectStates(ch chan<- prometheus.Metric, prefix string, desc *prometheus.Desc) {
	counts := make(map[string]float64, len(allStates))
	for _, s := range allStates {
		counts[s] = 0
	}

	items, err := kv.ListByPrefix(c.db, prefix)
	if err == nil {
		for key, val := range items {
			if strings.HasSuffix(key, "/state") {
				counts[val]++
			}
		}
	}

	for _, state := range allStates {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, counts[state], state)
	}
}

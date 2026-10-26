package watchdog

import (
	"fmt"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
)

const kindPublic = "public"

type publicProbe interface {
	LinkProblem(name string) string
	VxlanPorts(bridge string) ([]string, error)
}

type systemPublicProbe struct{}

func (systemPublicProbe) LinkProblem(name string) string { return linkProblem(name) }

func (systemPublicProbe) VxlanPorts(bridge string) ([]string, error) {
	return netif.BridgeVxlanPorts(bridge)
}

func CheckPublicSegment(cfg *configuration.Config, p publicProbe, n notify.Notifier) {
	bridge := cfg.PublicBridge()
	if problem := p.LinkProblem(bridge); problem != "" {
		n.Notify(kindPublic, bridge, problem+" (public bridge, interfaces.internet): the vpcs of this host have no public segment")
		return
	}
	ports, err := p.VxlanPorts(bridge)
	switch {
	case err != nil:
		n.Notify(kindPublic, bridge, fmt.Sprintf("vxlan ports unreadable: %v", err))
	case len(ports) == 0:
		n.Notify(kindPublic, bridge, "no vxlan port: the public segment is unreachable from this host")
	}
}

package watchdog

import (
	"fmt"
	"net"

	hostsvcapi "git.g3e.fr/syonad/two/internal/api/hostsvc"
	hostsvcclient "git.g3e.fr/syonad/two/internal/client/hostsvc"
	"git.g3e.fr/syonad/two/internal/hostsvc"
	"git.g3e.fr/syonad/two/internal/iptables"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/vpc"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

const kindHostSvc = "hostsvc"

type hostSvcProbe interface {
	Served() ([]string, error)
	ServiceAddress(vpc string) (bool, error)
	Redirect(vpc string) (bool, error)
}

type systemHostSvcProbe struct {
	client *hostsvcclient.Client
}

func newSystemHostSvcProbe() systemHostSvcProbe {
	return systemHostSvcProbe{client: hostsvcclient.New(hostsvcapi.SocketPath(hostsvcapi.DefaultRunDir))}
}

func (p systemHostSvcProbe) Served() ([]string, error) { return p.client.List() }

func (systemHostSvcProbe) ServiceAddress(name string) (bool, error) {
	want := net.ParseIP(hostsvc.ServiceIP)
	found := false
	err := netns.Call(name, func() error {
		ips, err := netif.LinkIPv4(vpc.HostSvcLink)
		if err != nil {
			return err
		}
		for _, ip := range ips {
			if ip.Equal(want) {
				found = true
			}
		}
		return nil
	})
	return found, err
}

func (systemHostSvcProbe) Redirect(name string) (bool, error) {
	var ok bool
	err := netns.Call(name, func() error {
		var err error
		ok, err = iptables.HasHostSvcRedirect(hostsvc.ServiceIP)
		return err
	})
	return ok, err
}

func CheckHostSvc(db *badger.DB, p hostSvcProbe, n notify.Notifier) error {
	pairs, err := kv.ListByPrefix(db, prefixVPC)
	if err != nil {
		return fmt.Errorf("watchdog: listing vpcs: %w", err)
	}
	var running []string
	for _, name := range resourceNames(pairs, prefixVPC) {
		if st, err := state.Get(db, prefixVPC+name); err == nil && st == state.Running {
			running = append(running, name)
		}
	}
	if len(running) == 0 {
		return nil
	}

	served, err := p.Served()
	if err != nil {
		n.Notify(kindHostSvc, hostsvcapi.Unit, fmt.Sprintf("unreachable, no vpc of this host gets dns or ntp: %v", err))
	} else {
		open := map[string]bool{}
		for _, name := range served {
			open[name] = true
		}
		for _, name := range running {
			if !open[name] {
				n.Notify(kindVPC, name, "not served by two-hostsvc: its vms get no dns nor ntp")
			}
		}
	}

	for _, name := range running {
		if ok, err := p.ServiceAddress(name); err != nil {
			n.Notify(kindVPC, name, fmt.Sprintf("%s on %s unreadable: %v", hostsvc.ServiceIP, vpc.HostSvcLink, err))
		} else if !ok {
			n.Notify(kindVPC, name, fmt.Sprintf("%s missing on %s (in netns)", hostsvc.ServiceIP, vpc.HostSvcLink))
		}
		if ok, err := p.Redirect(name); err != nil {
			n.Notify(kindVPC, name, fmt.Sprintf("dns and ntp redirection unreadable: %v", err))
		} else if !ok {
			n.Notify(kindVPC, name, fmt.Sprintf("dns and ntp redirection to %s missing or incomplete (chain %s)", hostsvc.ServiceIP, iptables.HostSvcChain))
		}
	}
	return nil
}

package watchdog

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	dhcpapi "git.g3e.fr/syonad/two/internal/api/dhcp"
	"git.g3e.fr/syonad/two/internal/dhcp"
	"git.g3e.fr/syonad/two/internal/dhcpbackend"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

type configFileReporter interface {
	ConfigPath(s dhcpbackend.Subnet) string
}

type stateReporter interface {
	State(s dhcpbackend.Subnet) (dhcpapi.State, string, error)
}

func expectedHosts(db *badger.DB, subnetName string) ([]dhcpapi.Host, error) {
	pairs, err := kv.ListByPrefix(db, prefixVM)
	if err != nil {
		return nil, fmt.Errorf("listing vms: %w", err)
	}

	hosts := make([]dhcpapi.Host, 0)
	for _, vmName := range resourceNames(pairs, prefixVM) {
		st, err := state.Get(db, prefixVM+vmName)
		if err != nil || st != state.Running {
			continue
		}

		vmHosts, err := expectedVMHosts(db, vmName, subnetName)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, vmHosts...)
	}

	dhcpapi.SortHosts(hosts)
	return hosts, nil
}

func expectedVMHosts(db *badger.DB, vmName, subnetName string) ([]dhcpapi.Host, error) {
	prefix := prefixVM + vmName + "/nic/"
	entries, err := kv.ListByPrefix(db, prefix)
	if err != nil {
		return nil, fmt.Errorf("listing nics of vm %s: %w", vmName, err)
	}

	indexes := make([]int, 0)
	for key := range entries {
		parts := strings.Split(strings.TrimPrefix(key, prefix), "/")
		if len(parts) != 2 || parts[1] != "subnet" {
			continue
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid nic index %q for vm %s", parts[0], vmName)
		}
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)

	hosts := make([]dhcpapi.Host, 0, len(indexes))
	for _, idx := range indexes {
		nic := fmt.Sprintf("%s%d/", prefix, idx)
		if entries[nic+"subnet"] != subnetName {
			continue
		}

		ip := entries[nic+"ip"]
		if ip == "" {
			return nil, fmt.Errorf("nic %d of vm %s has no ip", idx, vmName)
		}
		mac, err := dhcp.GetMACForIP(db, subnetName, ip)
		if err != nil {
			return nil, fmt.Errorf("get mac for ip %s: %w", ip, err)
		}

		hosts = append(hosts, dhcpapi.Host{
			MAC:          mac,
			IP:           ip,
			VM:           vmName,
			DefaultRoute: entries[nic+"primary"] == "true",
		})
	}
	return hosts, nil
}

func checkDHCP(db *badger.DB, name string, s dhcpbackend.Subnet, backend dhcpbackend.Backend, u unitChecker, n notify.Notifier) {
	if backend == nil {
		return
	}
	if r, ok := backend.(configFileReporter); ok {
		checkDHCPConfigFile(name, r.ConfigPath(s), n)
	}
	if r, ok := backend.(stateReporter); ok {
		checkDHCPState(db, name, s, r, n)
	}
	checkUnit(kindSubnet, name, backend.Unit(s), u, n)
}

func checkDHCPConfigFile(name, path string, n notify.Notifier) {
	if _, err := os.Stat(path); err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("dnsmasq config missing (%s): %v", path, err))
	}
}

func checkDHCPState(db *badger.DB, name string, s dhcpbackend.Subnet, r stateReporter, n notify.Notifier) {
	served, _, err := r.State(s)
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("dhcp server unreachable: %v", err))
		return
	}
	if served.Subnet == nil {
		n.Notify(kindSubnet, name, "dhcp server has no subnet configuration: it serves nothing")
	}

	expected, err := expectedHosts(db, name)
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("expected dhcp reservations unreadable in database: %v", err))
		return
	}

	expectedDigest, err := dhcpapi.Digest(dhcpapi.State{Hosts: expected})
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("expected dhcp reservations inconsistent in database: %v", err))
		return
	}
	servedDigest, err := dhcpapi.Digest(dhcpapi.State{Hosts: served.Hosts})
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("dhcp reservations reported by the server are inconsistent: %v", err))
		return
	}
	if expectedDigest == servedDigest {
		return
	}

	for _, gap := range reservationGaps(expected, served.Hosts) {
		n.Notify(kindSubnet, name, gap)
	}
}

func reservationGaps(expected, served []dhcpapi.Host) []string {
	index := func(hosts []dhcpapi.Host) map[string]dhcpapi.Host {
		byMAC := make(map[string]dhcpapi.Host, len(hosts))
		for _, h := range hosts {
			if c, err := dhcpapi.CanonicalHost(h); err == nil {
				byMAC[c.MAC] = c
			} else {
				byMAC[h.MAC] = h
			}
		}
		return byMAC
	}

	want, got := index(expected), index(served)

	gaps := make([]string, 0)
	for mac, h := range want {
		s, ok := got[mac]
		if !ok {
			gaps = append(gaps, fmt.Sprintf("dhcp reservation missing on the server: %s → %s (vm %s)", mac, h.IP, h.VM))
			continue
		}
		if s.IP != h.IP {
			gaps = append(gaps, fmt.Sprintf("dhcp reservation diverges for %s: server serves %s, database says %s (vm %s)", mac, s.IP, h.IP, h.VM))
		}
		if s.DefaultRoute != h.DefaultRoute {
			gaps = append(gaps, fmt.Sprintf("dhcp default route diverges for %s: server says %t, database says %t (vm %s)", mac, s.DefaultRoute, h.DefaultRoute, h.VM))
		}
	}
	for mac, h := range got {
		if _, ok := want[mac]; !ok {
			gaps = append(gaps, fmt.Sprintf("stale dhcp reservation on the server: %s → %s (vm %s)", mac, h.IP, h.VM))
		}
	}
	sort.Strings(gaps)
	return gaps
}

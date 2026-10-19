package migration

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

func MigrateVMMACs(db *badger.DB, log *slog.Logger) error {
	vms, err := kv.ListByPrefix(db, "vm/")
	if err != nil {
		return fmt.Errorf("list vm/: %w", err)
	}

	for _, prefix := range nicsWithoutMAC(vms) {
		name := strings.Split(prefix, "/")[1]
		if st, err := state.Get(db, "vm/"+name); err == nil && st == state.Deleted {
			continue
		}

		subnet, ip := vms[prefix+"subnet"], vms[prefix+"ip"]
		if ip == "" {
			return fmt.Errorf("%s has no ip: its mac cannot be recovered", strings.TrimSuffix(prefix, "/"))
		}
		mac, err := kv.GetFromDB(db, "subnet/"+subnet+"/dhcp/"+ip)
		if err != nil {
			return fmt.Errorf("%s: no mac for %s in subnet/%s/dhcp: %w", strings.TrimSuffix(prefix, "/"), ip, subnet, err)
		}
		if err := kv.AddInDB(db, prefix+"mac", mac); err != nil {
			return fmt.Errorf("store %smac: %w", prefix, err)
		}
		log.Info("vm nic mac migrated", "resource", strings.TrimSuffix(prefix, "/"), "mac", mac, "reason", "mac now provided at creation")
	}

	subnets, err := kv.ListByPrefix(db, "subnet/")
	if err != nil {
		return fmt.Errorf("list subnet/: %w", err)
	}
	for _, name := range subnetsWithMACTable(subnets) {
		if err := kv.DeleteInDB(db, "subnet/"+name+"/dhcp"); err != nil {
			return fmt.Errorf("delete subnet/%s/dhcp: %w", name, err)
		}
		log.Info("preloaded mac table removed", "resource", "subnet/"+name, "reason", "mac now provided at creation")
	}
	return nil
}

func nicsWithoutMAC(entries map[string]string) []string {
	var prefixes []string
	for key := range entries {
		parts := strings.Split(key, "/")
		if len(parts) != 5 || parts[2] != "nic" || parts[4] != "subnet" {
			continue
		}
		prefix := strings.TrimSuffix(key, "subnet")
		if _, ok := entries[prefix+"mac"]; !ok {
			prefixes = append(prefixes, prefix)
		}
	}
	sort.Strings(prefixes)
	return prefixes
}

func subnetsWithMACTable(entries map[string]string) []string {
	seen := make(map[string]bool)
	for key := range entries {
		parts := strings.SplitN(key, "/", 4)
		if len(parts) == 4 && parts[2] == "dhcp" {
			seen[parts[1]] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

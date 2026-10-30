package migration

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

func MigrateVMNICNames(db *badger.DB, log *slog.Logger) error {
	vms, err := kv.ListByPrefix(db, "vm/")
	if err != nil {
		return fmt.Errorf("list vm/: %w", err)
	}
	for _, prefix := range nicsWithoutName(vms) {
		name := "nic-" + strings.Split(prefix, "/")[3]
		if err := kv.AddInDB(db, prefix+"name", name); err != nil {
			return fmt.Errorf("store %sname: %w", prefix, err)
		}
		log.Info("vm nic name migrated", "resource", strings.TrimSuffix(prefix, "/"), "name", name, "reason", "interface name now required")
	}
	return nil
}

func nicsWithoutName(entries map[string]string) []string {
	var prefixes []string
	for key := range entries {
		parts := strings.Split(key, "/")
		if len(parts) != 5 || parts[2] != "nic" || parts[4] != "subnet" {
			continue
		}
		prefix := strings.TrimSuffix(key, "subnet")
		if _, ok := entries[prefix+"name"]; !ok {
			prefixes = append(prefixes, prefix)
		}
	}
	sort.Strings(prefixes)
	return prefixes
}

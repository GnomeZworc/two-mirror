package migration

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

var legacyDiskKey = regexp.MustCompile(`^vm/([^/]+)/disk/sd([a-z])$`)

func MigrateVMDisks(db *badger.DB, log *slog.Logger) error {
	entries, err := kv.ListByPrefix(db, "vm/")
	if err != nil {
		return fmt.Errorf("list vm/: %w", err)
	}

	for key, path := range entries {
		m := legacyDiskKey.FindStringSubmatch(key)
		if m == nil {
			continue
		}
		target := strings.TrimSuffix(key, "sd"+m[2]) + "xvd" + m[2]
		if existing, ok := entries[target]; ok && existing != path {
			return fmt.Errorf("migrate %s: %s already holds %q", key, target, existing)
		}
		if err := kv.AddInDB(db, target, path); err != nil {
			return fmt.Errorf("migrate %s: %w", key, err)
		}
		if err := kv.DeleteInDB(db, key); err != nil {
			return fmt.Errorf("delete legacy %s: %w", key, err)
		}
		log.Info("vm disk migrated", "resource", "vm/"+m[1], "from", "sd"+m[2], "to", "xvd"+m[2])
	}
	return nil
}

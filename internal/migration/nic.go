package migration

import (
	"fmt"
	"log/slog"
	"strings"

	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

// legacyNICKeys sont les clés d'interface d'avant le multi-subnet, portées
// directement par la VM.
var legacyNICKeys = []string{"subnet", "ip", "tap_id"}

// MigrateVMNICs déplace les clés d'interface de vm/<name>/<clé> vers
// vm/<name>/nic/0/<clé> et marque cette interface comme primaire.
//
// Idempotente : une VM possédant déjà des clés nic/ est laissée telle quelle.
// Sans cette migration, toute VM créée avant le passage au multi-subnet
// deviendrait illisible par loadVM.
func MigrateVMNICs(db *badger.DB, log *slog.Logger) error {
	entries, err := kv.ListByPrefix(db, "vm/")
	if err != nil {
		return fmt.Errorf("list vm/: %w", err)
	}

	for _, name := range vmsToMigrate(entries) {
		for _, key := range legacyNICKeys {
			value, ok := entries["vm/"+name+"/"+key]
			if !ok {
				continue
			}
			if err := kv.AddInDB(db, "vm/"+name+"/nic/0/"+key, value); err != nil {
				return fmt.Errorf("migrate %s of vm %s: %w", key, name, err)
			}
			if err := kv.DeleteInDB(db, "vm/"+name+"/"+key); err != nil {
				return fmt.Errorf("delete legacy %s of vm %s: %w", key, name, err)
			}
		}
		if err := kv.AddInDB(db, "vm/"+name+"/nic/0/primary", "true"); err != nil {
			return fmt.Errorf("mark nic 0 primary for vm %s: %w", name, err)
		}
		log.Info("vm nics migrated", "resource", "vm/"+name, "reason", "legacy single interface")
	}
	return nil
}

// vmsToMigrate retourne les VM portant l'ancien schéma et aucune clé nic/.
func vmsToMigrate(entries map[string]string) []string {
	legacy := make(map[string]bool)
	migrated := make(map[string]bool)

	for key := range entries {
		parts := strings.Split(key, "/")
		if len(parts) < 3 || parts[0] != "vm" {
			continue
		}
		name := parts[1]
		switch {
		case len(parts) == 3 && (parts[2] == "subnet" || parts[2] == "ip" || parts[2] == "tap_id"):
			legacy[name] = true
		case parts[2] == "nic":
			migrated[name] = true
		}
	}

	var names []string
	for name := range legacy {
		if !migrated[name] {
			names = append(names, name)
		}
	}
	return names
}

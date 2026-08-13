// Package migration met la DB au format attendu par la version courante de
// l'agent. Les migrations sont idempotentes et jouées au démarrage.
package migration

import (
	"fmt"
	"log/slog"
	"strings"

	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

// prefixes énumère les familles de ressources portant une clé /state.
var prefixes = []string{"vpc/", "subnet/", "vm/"}

// legacyStates traduit l'ancien vocabulaire (états VPC/subnet d'avant
// l'unification, et états VM start/stop) vers l'enum courant.
var legacyStates = map[string]state.State{
	"created":  state.Running,
	"started":  state.Running,
	"starting": state.Creating,
	"stopping": state.Deleting,
	"stopped":  state.Deleted,
}

// MigrateStates convertit les valeurs de state héritées vers l'enum courant,
// puis marque en erreur les ressources restées dans un état transitoire.
//
// La réconciliation est sûre au démarrage : la queue worker est en mémoire et
// vide à ce moment, donc aucune commande n'est en cours. Une ressource en
// creating/deleting est nécessairement orpheline d'un arrêt de l'agent — sans
// ce passage en error, elle resterait indéfiniment non supprimable.
func MigrateStates(db *badger.DB, log *slog.Logger) error {
	for _, prefix := range prefixes {
		entries, err := kv.ListByPrefix(db, prefix)
		if err != nil {
			return fmt.Errorf("list %s: %w", prefix, err)
		}
		for key, value := range entries {
			if !strings.HasSuffix(key, "/state") {
				continue
			}
			resource := strings.TrimSuffix(key, "/state")
			target, reason := targetState(value)
			if target == "" {
				continue
			}
			if err := state.Set(db, resource, target); err != nil {
				return fmt.Errorf("migrate %s: %w", key, err)
			}
			log.Info("state migrated",
				"resource", resource, "from", value, "to", string(target), "reason", reason)
		}
	}
	return nil
}

// targetState retourne l'état vers lequel migrer une valeur brute, ou "" si
// elle doit rester inchangée.
func targetState(value string) (state.State, string) {
	if legacy, ok := legacyStates[value]; ok {
		// Un état hérité transitoire est orphelin au même titre qu'un état
		// transitoire courant : on applique la réconciliation directement.
		if state.IsTransient(legacy) {
			return state.Error, "orphaned legacy state"
		}
		return legacy, "legacy state"
	}

	current, err := state.Parse(value)
	if err != nil {
		// Valeur inconnue : ni l'ancien vocabulaire, ni le nouveau. On la
		// bascule en error plutôt que de la laisser bloquer l'agent — la
		// ressource reste visible et supprimable.
		return state.Error, "unknown state"
	}
	if state.IsTransient(current) {
		return state.Error, "orphaned transient state"
	}
	return "", ""
}

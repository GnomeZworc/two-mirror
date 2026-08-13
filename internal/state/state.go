// Package state définit les états possibles d'une ressource (VPC, subnet, VM)
// et centralise leur lecture/écriture en DB.
package state

import (
	"fmt"

	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

type State string

const (
	Creating State = "creating"
	Running  State = "running"
	Error    State = "error"
	Deleting State = "deleting"
	Deleted  State = "deleted"
)

// All retourne tous les états valides, dans l'ordre du cycle de vie.
func All() []State {
	return []State{Creating, Running, Error, Deleting, Deleted}
}

// Parse convertit une valeur brute lue en DB en State.
func Parse(s string) (State, error) {
	for _, valid := range All() {
		if State(s) == valid {
			return valid, nil
		}
	}
	return "", fmt.Errorf("unknown state %q", s)
}

// CanDelete indique si une ressource dans cet état peut être supprimée.
// Une ressource en cours de création ou déjà en cours de suppression ne l'est pas ;
// une ressource en erreur l'est, pour permettre le nettoyage d'un échec partiel.
func CanDelete(s State) bool {
	return s == Running || s == Error
}

// IsTransient indique si l'état suppose une commande en cours d'exécution.
// Au démarrage de l'agent, la queue worker est vide : une ressource dans un état
// transitoire est donc orpheline.
func IsTransient(s State) bool {
	return s == Creating || s == Deleting
}

// Get lit l'état d'une ressource. prefix est la racine de la ressource,
// sans le suffixe /state (ex: "vpc/vpc-1").
func Get(db *badger.DB, prefix string) (State, error) {
	raw, err := kv.GetFromDB(db, prefix+"/state")
	if err != nil {
		return "", err
	}
	return Parse(raw)
}

// Set écrit l'état d'une ressource. prefix suit la même convention que Get.
func Set(db *badger.DB, prefix string, s State) error {
	if _, err := Parse(string(s)); err != nil {
		return err
	}
	return kv.AddInDB(db, prefix+"/state", string(s))
}

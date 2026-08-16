package migration

import (
	"io"
	"log/slog"
	"testing"

	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

func newTestDB(t *testing.T) *badger.DB {
	t.Helper()
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	return db
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestMigrateStates_LegacyMapping(t *testing.T) {
	db := newTestDB(t)
	cases := map[string]struct {
		resource string
		from     string
		want     state.State
	}{
		"vpc created":    {"vpc/vpc-1", "created", state.Running},
		"subnet created": {"subnet/sn-1", "created", state.Running},
		"vm started":     {"vm/vm-1", "started", state.Running},
		"vm stopped":     {"vm/vm-2", "stopped", state.Deleted},
		// Les états transitoires hérités sont orphelins après un redémarrage.
		"vm starting":     {"vm/vm-3", "starting", state.Error},
		"vm stopping":     {"vm/vm-4", "stopping", state.Error},
		"vpc creating":    {"vpc/vpc-2", "creating", state.Error},
		"subnet deleting": {"subnet/sn-2", "deleting", state.Error},
	}
	for _, c := range cases {
		kv.AddInDB(db, c.resource+"/state", c.from)
	}

	if err := MigrateStates(db, discardLogger()); err != nil {
		t.Fatalf("MigrateStates a échoué : %v", err)
	}

	for name, c := range cases {
		got, err := state.Get(db, c.resource)
		if err != nil {
			t.Errorf("%s : Get a échoué : %v", name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s : %q → %q, attendu %q", name, c.from, got, c.want)
		}
	}
}

func TestMigrateStates_StableStatesUntouched(t *testing.T) {
	db := newTestDB(t)
	stable := map[string]state.State{
		"vpc/vpc-1":   state.Running,
		"subnet/sn-1": state.Deleted,
		"vm/vm-1":     state.Error,
	}
	for resource, s := range stable {
		if err := state.Set(db, resource, s); err != nil {
			t.Fatalf("préparation du test : %v", err)
		}
	}

	if err := MigrateStates(db, discardLogger()); err != nil {
		t.Fatalf("MigrateStates a échoué : %v", err)
	}

	for resource, want := range stable {
		got, err := state.Get(db, resource)
		if err != nil {
			t.Fatalf("Get(%s) a échoué : %v", resource, err)
		}
		if got != want {
			t.Errorf("%s : état modifié %q, attendu %q", resource, got, want)
		}
	}
}

func TestMigrateStates_Idempotent(t *testing.T) {
	db := newTestDB(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "created")
	kv.AddInDB(db, "vm/vm-1/state", "starting")

	if err := MigrateStates(db, discardLogger()); err != nil {
		t.Fatalf("premier passage : %v", err)
	}
	first := map[string]state.State{}
	for _, r := range []string{"vpc/vpc-1", "vm/vm-1"} {
		s, err := state.Get(db, r)
		if err != nil {
			t.Fatalf("Get(%s) : %v", r, err)
		}
		first[r] = s
	}

	if err := MigrateStates(db, discardLogger()); err != nil {
		t.Fatalf("second passage : %v", err)
	}
	for r, want := range first {
		got, err := state.Get(db, r)
		if err != nil {
			t.Fatalf("Get(%s) : %v", r, err)
		}
		if got != want {
			t.Errorf("%s : le second passage a modifié l'état (%q → %q)", r, want, got)
		}
	}
}

func TestMigrateStates_UnknownValueBecomesError(t *testing.T) {
	db := newTestDB(t)
	kv.AddInDB(db, "vpc/vpc-corrompu/state", "n'importe quoi")

	if err := MigrateStates(db, discardLogger()); err != nil {
		t.Fatalf("MigrateStates a échoué : %v", err)
	}

	got, err := state.Get(db, "vpc/vpc-corrompu")
	if err != nil {
		t.Fatalf("Get a échoué : %v", err)
	}
	if got != state.Error {
		t.Errorf("une valeur inconnue devrait devenir %q, obtenu %q", state.Error, got)
	}
}

func TestMigrateStates_IgnoresNonStateKeys(t *testing.T) {
	db := newTestDB(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "created")
	kv.AddInDB(db, "vpc/vpc-1/cidr", "10.0.0.0/16")
	kv.AddInDB(db, "subnet/sn-1/local_iface", "br-vms")
	kv.AddInDB(db, "vm/vm-1/disk/sda", "/data/root.qcow2")

	if err := MigrateStates(db, discardLogger()); err != nil {
		t.Fatalf("MigrateStates a échoué : %v", err)
	}

	untouched := map[string]string{
		"vpc/vpc-1/cidr":          "10.0.0.0/16",
		"subnet/sn-1/local_iface": "br-vms",
		"vm/vm-1/disk/sda":        "/data/root.qcow2",
	}
	for key, want := range untouched {
		got, err := kv.GetFromDB(db, key)
		if err != nil {
			t.Errorf("la clé %s devrait exister : %v", key, err)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, attendu %q", key, got, want)
		}
	}
	// Aucune clé /state parasite ne doit apparaître sur ces ressources.
	if _, err := kv.GetFromDB(db, "vm/vm-1/state"); err == nil {
		t.Error("aucune clé state ne devrait être créée pour vm-1")
	}
}

func TestMigrateStates_EmptyDB(t *testing.T) {
	db := newTestDB(t)
	if err := MigrateStates(db, discardLogger()); err != nil {
		t.Fatalf("MigrateStates devrait réussir sur une DB vide : %v", err)
	}
}

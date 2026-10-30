package migration

import (
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func TestMigrateVMNICNames_NamesEveryInterfaceByItsIndex(t *testing.T) {
	db := newTestDB(t)
	seedPreloadedVM(t, db, "vm-1", "running", map[int][2]string{0: {"sn-a", "10.0.0.10"}, 1: {"sn-b", "10.0.1.10"}})
	if err := MigrateVMNICNames(db, discardLogger()); err != nil {
		t.Fatal(err)
	}
	assertKey(t, db, "vm/vm-1/nic/0/name", "nic-0")
	assertKey(t, db, "vm/vm-1/nic/1/name", "nic-1")
}

func TestMigrateVMNICNames_KeepsAnExistingName(t *testing.T) {
	db := newTestDB(t)
	seedPreloadedVM(t, db, "vm-1", "running", map[int][2]string{0: {"sn-a", "10.0.0.10"}, 1: {"sn-b", "10.0.1.10"}})
	kv.AddInDB(db, "vm/vm-1/nic/0/name", "nic-a1b2")
	if err := MigrateVMNICNames(db, discardLogger()); err != nil {
		t.Fatal(err)
	}
	assertKey(t, db, "vm/vm-1/nic/0/name", "nic-a1b2")
	assertKey(t, db, "vm/vm-1/nic/1/name", "nic-1")
}

func TestMigrateVMNICNames_IsIdempotent(t *testing.T) {
	db := newTestDB(t)
	seedPreloadedVM(t, db, "vm-1", "running", map[int][2]string{0: {"sn-a", "10.0.0.10"}})
	for i := 0; i < 2; i++ {
		if err := MigrateVMNICNames(db, discardLogger()); err != nil {
			t.Fatalf("passage %d : %v", i+1, err)
		}
	}
	assertKey(t, db, "vm/vm-1/nic/0/name", "nic-0")
}

func TestMigrateVMNICNames_IgnoresKeysOutsideInterfaces(t *testing.T) {
	db := newTestDB(t)
	kv.AddInDB(db, "vm/vm-1/state", "running")
	kv.AddInDB(db, "vm/vm-1/disk/vda", "/data/vda.qcow2")
	if err := MigrateVMNICNames(db, discardLogger()); err != nil {
		t.Fatal(err)
	}
	entries, _ := kv.ListByPrefix(db, "vm/vm-1/")
	if len(entries) != 2 {
		t.Errorf("clés après migration : %v", entries)
	}
}

package migration

import (
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func TestMigrateVMDisks_RenamesSCSIDisks(t *testing.T) {
	db := newNICDB(t)
	kv.AddInDB(db, "vm/i-test1/disk/vda", "/v/root.qcow2")
	kv.AddInDB(db, "vm/i-test1/disk/sda", "/v/a.qcow2")
	kv.AddInDB(db, "vm/i-test1/disk/sdc", "/v/c.qcow2")
	kv.AddInDB(db, "vm/i-test2/disk/sdb", "/v/b.qcow2")

	if err := MigrateVMDisks(db, quietLog()); err != nil {
		t.Fatalf("MigrateVMDisks : %v", err)
	}

	got, err := kv.ListByPrefix(db, "vm/")
	if err != nil {
		t.Fatalf("ListByPrefix : %v", err)
	}
	want := map[string]string{
		"vm/i-test1/disk/vda":  "/v/root.qcow2",
		"vm/i-test1/disk/xvda": "/v/a.qcow2",
		"vm/i-test1/disk/xvdc": "/v/c.qcow2",
		"vm/i-test2/disk/xvdb": "/v/b.qcow2",
	}
	if len(got) != len(want) {
		t.Errorf("clés après migration : %v, attendu %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %q, attendu %q", key, got[key], value)
		}
	}
}

func TestMigrateVMDisks_Idempotent(t *testing.T) {
	db := newNICDB(t)
	kv.AddInDB(db, "vm/i-test1/disk/sda", "/v/a.qcow2")

	for i := 0; i < 2; i++ {
		if err := MigrateVMDisks(db, quietLog()); err != nil {
			t.Fatalf("passe %d : %v", i, err)
		}
	}
	if got, _ := kv.GetFromDB(db, "vm/i-test1/disk/xvda"); got != "/v/a.qcow2" {
		t.Errorf("xvda = %q après deux passes", got)
	}
}

func TestMigrateVMDisks_LeavesOtherKeys(t *testing.T) {
	db := newNICDB(t)
	kv.AddInDB(db, "vm/i-test1/nic/0/sda", "x")
	kv.AddInDB(db, "vm/i-test1/disk/sdaa", "/v/x.qcow2")

	if err := MigrateVMDisks(db, quietLog()); err != nil {
		t.Fatalf("MigrateVMDisks : %v", err)
	}
	for _, key := range []string{"vm/i-test1/nic/0/sda", "vm/i-test1/disk/sdaa"} {
		if _, err := kv.GetFromDB(db, key); err != nil {
			t.Errorf("%s ne devait pas être touchée : %v", key, err)
		}
	}
}

func TestMigrateVMDisks_RefusesConflict(t *testing.T) {
	db := newNICDB(t)
	kv.AddInDB(db, "vm/i-test1/disk/sda", "/v/a.qcow2")
	kv.AddInDB(db, "vm/i-test1/disk/xvda", "/v/other.qcow2")

	if err := MigrateVMDisks(db, quietLog()); err == nil {
		t.Fatal("un conflit sda/xvda doit faire échouer la migration")
	}
	if got, _ := kv.GetFromDB(db, "vm/i-test1/disk/sda"); got != "/v/a.qcow2" {
		t.Errorf("sda ne doit pas être supprimée en cas de conflit : %q", got)
	}
}

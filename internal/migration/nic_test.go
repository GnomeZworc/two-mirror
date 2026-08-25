package migration

import (
	"io"
	"log/slog"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

func newNICDB(t *testing.T) *badger.DB {
	t.Helper()
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	return db
}

func quietLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func seedLegacyVM(t *testing.T, db *badger.DB, name string) {
	t.Helper()
	kv.AddInDB(db, "vm/"+name+"/state", "running")
	kv.AddInDB(db, "vm/"+name+"/subnet", "sn-000001")
	kv.AddInDB(db, "vm/"+name+"/ip", "10.1.1.2")
	kv.AddInDB(db, "vm/"+name+"/tap_id", "12345678")
}

func TestMigrateVMNICs_MovesLegacyKeys(t *testing.T) {
	db := newNICDB(t)
	seedLegacyVM(t, db, "i-test1")

	if err := MigrateVMNICs(db, quietLog()); err != nil {
		t.Fatalf("MigrateVMNICs : %v", err)
	}

	want := map[string]string{
		"vm/i-test1/nic/0/subnet":  "sn-000001",
		"vm/i-test1/nic/0/ip":      "10.1.1.2",
		"vm/i-test1/nic/0/tap_id":  "12345678",
		"vm/i-test1/nic/0/primary": "true",
	}
	for key, expected := range want {
		got, err := kv.GetFromDB(db, key)
		if err != nil {
			t.Errorf("clé %s absente : %v", key, err)
			continue
		}
		if got != expected {
			t.Errorf("%s = %q, attendu %q", key, got, expected)
		}
	}
}

func TestMigrateVMNICs_RemovesLegacyKeys(t *testing.T) {
	db := newNICDB(t)
	seedLegacyVM(t, db, "i-test1")

	if err := MigrateVMNICs(db, quietLog()); err != nil {
		t.Fatalf("MigrateVMNICs : %v", err)
	}

	for _, key := range []string{"vm/i-test1/subnet", "vm/i-test1/ip", "vm/i-test1/tap_id"} {
		if _, err := kv.GetFromDB(db, key); err == nil {
			t.Errorf("clé héritée %s toujours présente", key)
		}
	}
}

func TestMigrateVMNICs_PreservesOtherKeys(t *testing.T) {
	db := newNICDB(t)
	seedLegacyVM(t, db, "i-test1")
	kv.AddInDB(db, "vm/i-test1/memory", "2048")
	kv.AddInDB(db, "vm/i-test1/disk/vda", "/data/root.qcow2")

	if err := MigrateVMNICs(db, quietLog()); err != nil {
		t.Fatalf("MigrateVMNICs : %v", err)
	}

	if got, _ := kv.GetFromDB(db, "vm/i-test1/memory"); got != "2048" {
		t.Errorf("memory altérée : %q", got)
	}
	if got, _ := kv.GetFromDB(db, "vm/i-test1/disk/vda"); got != "/data/root.qcow2" {
		t.Errorf("disque altéré : %q", got)
	}
}

func TestMigrateVMNICs_Idempotent(t *testing.T) {
	db := newNICDB(t)
	seedLegacyVM(t, db, "i-test1")

	if err := MigrateVMNICs(db, quietLog()); err != nil {
		t.Fatalf("premier passage : %v", err)
	}
	before, _ := kv.ListByPrefix(db, "vm/i-test1/")
	if err := MigrateVMNICs(db, quietLog()); err != nil {
		t.Fatalf("second passage : %v", err)
	}
	after, _ := kv.ListByPrefix(db, "vm/i-test1/")

	if len(before) != len(after) {
		t.Errorf("le second passage a modifié la base : %d clés puis %d", len(before), len(after))
	}
	for key, value := range before {
		if after[key] != value {
			t.Errorf("%s : %q devenu %q", key, value, after[key])
		}
	}
}

func TestMigrateVMNICs_LeavesMigratedVMsAlone(t *testing.T) {
	db := newNICDB(t)
	kv.AddInDB(db, "vm/i-multi/state", "running")
	kv.AddInDB(db, "vm/i-multi/nic/0/subnet", "sn-000001")
	kv.AddInDB(db, "vm/i-multi/nic/0/primary", "true")
	kv.AddInDB(db, "vm/i-multi/nic/1/subnet", "sn-000002")

	if err := MigrateVMNICs(db, quietLog()); err != nil {
		t.Fatalf("MigrateVMNICs : %v", err)
	}

	if got, _ := kv.GetFromDB(db, "vm/i-multi/nic/1/subnet"); got != "sn-000002" {
		t.Errorf("la seconde interface a été perdue : %q", got)
	}
	if _, err := kv.GetFromDB(db, "vm/i-multi/nic/0/primary"); err != nil {
		t.Error("la primaire existante a été perdue")
	}
}

func TestMigrateVMNICs_EmptyDB(t *testing.T) {
	if err := MigrateVMNICs(newNICDB(t), quietLog()); err != nil {
		t.Errorf("une base vide ne doit pas échouer : %v", err)
	}
}

package migration

import (
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

func seedPreloadedVM(t *testing.T, db *badger.DB, name, state string, nics map[int][2]string) {
	t.Helper()
	kv.AddInDB(db, "vm/"+name+"/state", state)
	for idx, nic := range nics {
		prefix := "vm/" + name + "/nic/" + string(rune('0'+idx)) + "/"
		kv.AddInDB(db, prefix+"subnet", nic[0])
		kv.AddInDB(db, prefix+"ip", nic[1])
	}
}

func seedMACTable(t *testing.T, db *badger.DB, subnet string, macs map[string]string) {
	t.Helper()
	kv.AddInDB(db, "subnet/"+subnet+"/state", "running")
	kv.AddInDB(db, "subnet/"+subnet+"/cidr", "10.0.0.0/24")
	for ip, mac := range macs {
		kv.AddInDB(db, "subnet/"+subnet+"/dhcp/"+ip, mac)
	}
}

func assertKey(t *testing.T, db *badger.DB, key, want string) {
	t.Helper()
	got, err := kv.GetFromDB(db, key)
	if err != nil || got != want {
		t.Errorf("%s = %q (%v), attendu %q", key, got, err, want)
	}
}

func assertNoMACTable(t *testing.T, db *badger.DB) {
	t.Helper()
	entries, err := kv.ListByPrefix(db, "subnet/")
	if err != nil {
		t.Fatalf("ListByPrefix : %v", err)
	}
	for key := range entries {
		if strings.Contains(key, "/dhcp/") {
			t.Errorf("clé de l'ancienne table restante : %s", key)
		}
	}
}

func TestMigrateVMMACs_CopiesTheMACOfEveryInterfaceThenDropsTheTable(t *testing.T) {
	db := newNICDB(t)
	seedMACTable(t, db, "sn-a", map[string]string{"10.1.0.10": "00:22:33:00:00:0A", "10.1.0.11": "00:22:33:00:00:0B"})
	seedMACTable(t, db, "sn-b", map[string]string{"10.2.0.10": "00:22:33:00:00:0A"})
	seedPreloadedVM(t, db, "vm-1", "running", map[int][2]string{0: {"sn-a", "10.1.0.10"}, 1: {"sn-b", "10.2.0.10"}})

	if err := MigrateVMMACs(db, quietLog()); err != nil {
		t.Fatalf("MigrateVMMACs : %v", err)
	}

	assertKey(t, db, "vm/vm-1/nic/0/mac", "00:22:33:00:00:0A")
	assertKey(t, db, "vm/vm-1/nic/1/mac", "00:22:33:00:00:0A")
	assertNoMACTable(t, db)
	assertKey(t, db, "subnet/sn-a/cidr", "10.0.0.0/24")
}

func TestMigrateVMMACs_KeepsAnExistingMAC(t *testing.T) {
	db := newNICDB(t)
	seedMACTable(t, db, "sn-a", map[string]string{"10.1.0.10": "00:22:33:00:00:0A"})
	seedPreloadedVM(t, db, "vm-1", "running", map[int][2]string{0: {"sn-a", "10.1.0.10"}})
	kv.AddInDB(db, "vm/vm-1/nic/0/mac", "52:54:00:ab:cd:ef")

	if err := MigrateVMMACs(db, quietLog()); err != nil {
		t.Fatalf("MigrateVMMACs : %v", err)
	}

	assertKey(t, db, "vm/vm-1/nic/0/mac", "52:54:00:ab:cd:ef")
	assertNoMACTable(t, db)
}

func TestMigrateVMMACs_IsIdempotent(t *testing.T) {
	db := newNICDB(t)
	seedMACTable(t, db, "sn-a", map[string]string{"10.1.0.10": "00:22:33:00:00:0A"})
	seedPreloadedVM(t, db, "vm-1", "running", map[int][2]string{0: {"sn-a", "10.1.0.10"}})

	for i := range 2 {
		if err := MigrateVMMACs(db, quietLog()); err != nil {
			t.Fatalf("passage %d : %v", i+1, err)
		}
	}
	assertKey(t, db, "vm/vm-1/nic/0/mac", "00:22:33:00:00:0A")
}

func TestMigrateVMMACs_IgnoresDeletedVMs(t *testing.T) {
	db := newNICDB(t)
	seedPreloadedVM(t, db, "vm-gone", "deleted", map[int][2]string{0: {"sn-gone", "10.9.0.10"}})

	if err := MigrateVMMACs(db, quietLog()); err != nil {
		t.Fatalf("une VM supprimée dont le subnet a disparu ne doit pas bloquer le démarrage : %v", err)
	}
	if _, err := kv.GetFromDB(db, "vm/vm-gone/nic/0/mac"); err == nil {
		t.Error("aucune mac ne doit être inventée pour une VM supprimée")
	}
}

func TestMigrateVMMACs_AMissingEntryFailsAndKeepsTheTable(t *testing.T) {
	db := newNICDB(t)
	seedMACTable(t, db, "sn-a", map[string]string{"10.1.0.10": "00:22:33:00:00:0A"})
	seedPreloadedVM(t, db, "vm-1", "running", map[int][2]string{0: {"sn-a", "10.1.0.99"}})

	err := MigrateVMMACs(db, quietLog())
	if err == nil || !strings.Contains(err.Error(), "vm/vm-1/nic/0") {
		t.Fatalf("erreur = %v, attendu une erreur nommant vm/vm-1/nic/0", err)
	}
	assertKey(t, db, "subnet/sn-a/dhcp/10.1.0.10", "00:22:33:00:00:0A")
}

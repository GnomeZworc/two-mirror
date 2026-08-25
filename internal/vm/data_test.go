package vm

import (
	"fmt"
	"strconv"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

func newVMInDB(t *testing.T) *badger.DB {
	t.Helper()
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })

	kv.AddInDB(db, "vm/vm-1/nic/0/subnet", "sn-000001")
	kv.AddInDB(db, "vm/vm-1/nic/0/primary", "true")
	kv.AddInDB(db, "subnet/sn-000001/vpc", "vp-admin")
	kv.AddInDB(db, "subnet/sn-000001/interface_ip", "10.1.1.1")
	kv.AddInDB(db, "subnet/sn-000001/dhcp/10.1.1.2", "00:22:33:00:01:02")
	kv.AddInDB(db, "vm/vm-1/nic/0/ip", "10.1.1.2")
	kv.AddInDB(db, "vm/vm-1/metadata_port", "8081")
	kv.AddInDB(db, "vm/vm-1/disk/vda", "/data/root.qcow2")
	kv.AddInDB(db, "vm/vm-1/memory", "2048")
	kv.AddInDB(db, "vm/vm-1/cpus", "2")
	return db
}

func TestLoadVM_NoDocuments(t *testing.T) {
	db := newVMInDB(t)

	d, err := loadVM(db, "vm-1")
	if err != nil {
		t.Fatalf("loadVM : %v", err)
	}
	if d.documents != nil {
		t.Errorf("aucun document en base, la map doit rester nil : %v", d.documents)
	}
}

func TestLoadVM_ReadsDocuments(t *testing.T) {
	db := newVMInDB(t)
	userData := "#cloud-config\npackages:\n  - nginx\n"
	kv.AddInDB(db, "vm/vm-1/metadata/user-data", userData)
	kv.AddInDB(db, "vm/vm-1/metadata/network-config", "version: 2\n")

	d, err := loadVM(db, "vm-1")
	if err != nil {
		t.Fatalf("loadVM : %v", err)
	}
	if len(d.documents) != 2 {
		t.Fatalf("2 documents attendus, obtenu %d : %v", len(d.documents), d.documents)
	}
	if d.documents["user-data"] != userData {
		t.Errorf("user-data altéré au passage en base :\nattendu %q\nobtenu  %q", userData, d.documents["user-data"])
	}
	if d.documents["network-config"] != "version: 2\n" {
		t.Errorf("network-config inattendu : %q", d.documents["network-config"])
	}
}

func TestLoadVM_DocumentKeysAreStrippedOfPrefix(t *testing.T) {
	db := newVMInDB(t)
	kv.AddInDB(db, "vm/vm-1/metadata/vendor-data", "v")

	d, err := loadVM(db, "vm-1")
	if err != nil {
		t.Fatalf("loadVM : %v", err)
	}
	for key := range d.documents {
		if key != "vendor-data" {
			t.Errorf("clé de document attendue %q, obtenue %q — le préfixe doit être retiré", "vendor-data", key)
		}
	}
}

func TestLoadVM_EmptyDocumentIsPreserved(t *testing.T) {
	db := newVMInDB(t)
	kv.AddInDB(db, "vm/vm-1/metadata/user-data", "")

	d, err := loadVM(db, "vm-1")
	if err != nil {
		t.Fatalf("loadVM : %v", err)
	}
	content, ok := d.documents["user-data"]
	if !ok {
		t.Fatal("un document vide doit survivre au passage en base : c'est une demande de ne rien servir")
	}
	if content != "" {
		t.Errorf("contenu attendu vide, obtenu %q", content)
	}
}

// --- multi-interfaces ---

func addNIC(t *testing.T, db *badger.DB, idx int, subnet, ip, mac string, primary bool) {
	t.Helper()
	prefix := fmt.Sprintf("vm/vm-1/nic/%d/", idx)
	kv.AddInDB(db, prefix+"subnet", subnet)
	kv.AddInDB(db, prefix+"ip", ip)
	if primary {
		kv.AddInDB(db, prefix+"primary", "true")
	}
	kv.AddInDB(db, "subnet/"+subnet+"/vpc", "vp-admin")
	kv.AddInDB(db, "subnet/"+subnet+"/interface_ip", "10.1.1.1")
	kv.AddInDB(db, "subnet/"+subnet+"/dhcp/"+ip, mac)
}

func TestLoadVM_SingleNIC(t *testing.T) {
	d, err := loadVM(newVMInDB(t), "vm-1")
	if err != nil {
		t.Fatalf("loadVM : %v", err)
	}
	if len(d.nics) != 1 {
		t.Fatalf("1 interface attendue, obtenu %d", len(d.nics))
	}
	if !d.primary().primary || d.primary().ip != "10.1.1.2" {
		t.Errorf("primaire inattendue : %+v", d.primary())
	}
}

func TestLoadVM_MultipleNICsSortedByIndex(t *testing.T) {
	db := newVMInDB(t)
	addNIC(t, db, 2, "sn-000003", "10.3.0.9", "00:22:33:00:03:09", false)
	addNIC(t, db, 1, "sn-000002", "10.2.0.5", "00:22:33:00:02:05", false)

	d, err := loadVM(db, "vm-1")
	if err != nil {
		t.Fatalf("loadVM : %v", err)
	}
	if len(d.nics) != 3 {
		t.Fatalf("3 interfaces attendues, obtenu %d", len(d.nics))
	}
	for i, n := range d.nics {
		if n.index != i {
			t.Errorf("interface en position %d porte l'index %d — l'ordre détermine le slot PCI", i, n.index)
		}
	}
}

func TestLoadVM_TapAllocatedPerNIC(t *testing.T) {
	db := newVMInDB(t)
	addNIC(t, db, 1, "sn-000002", "10.2.0.5", "00:22:33:00:02:05", false)

	d, err := loadVM(db, "vm-1")
	if err != nil {
		t.Fatalf("loadVM : %v", err)
	}
	if d.nics[0].tapID == d.nics[1].tapID {
		t.Errorf("deux interfaces partagent le tap %d", d.nics[0].tapID)
	}
	for _, n := range d.nics {
		stored, err := kv.GetFromDB(db, fmt.Sprintf("vm/vm-1/nic/%d/tap_id", n.index))
		if err != nil {
			t.Errorf("tap_id de l'interface %d non persisté : %v", n.index, err)
			continue
		}
		if stored != strconv.Itoa(n.tapID) {
			t.Errorf("tap_id de l'interface %d : %q en base, %d en mémoire", n.index, stored, n.tapID)
		}
	}
}

func TestLoadVM_NoPrimaryIsAnError(t *testing.T) {
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	kv.AddInDB(db, "vm/vm-1/metadata_port", "8081")
	kv.AddInDB(db, "vm/vm-1/disk/vda", "/data/root.qcow2")
	kv.AddInDB(db, "vm/vm-1/memory", "2048")
	kv.AddInDB(db, "vm/vm-1/cpus", "2")
	addNIC(t, db, 0, "sn-000001", "10.1.1.2", "00:22:33:00:01:02", false)

	if _, err := loadVM(db, "vm-1"); err == nil {
		t.Error("aucune interface primaire : loadVM doit échouer plutôt que de laisser StartVM choisir au hasard")
	}
}

func TestLoadVM_TwoPrimariesIsAnError(t *testing.T) {
	db := newVMInDB(t)
	addNIC(t, db, 1, "sn-000002", "10.2.0.5", "00:22:33:00:02:05", true)

	if _, err := loadVM(db, "vm-1"); err == nil {
		t.Error("deux interfaces primaires doivent être refusées")
	}
}

func TestLoadVM_NoNICIsAnError(t *testing.T) {
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	kv.AddInDB(db, "vm/vm-1/memory", "2048")

	if _, err := loadVM(db, "vm-1"); err == nil {
		t.Error("une VM sans interface doit être refusée")
	}
}

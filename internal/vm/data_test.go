package vm

import (
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

func newVMInDB(t *testing.T) *badger.DB {
	t.Helper()
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })

	kv.AddInDB(db, "vm/vm-1/subnet", "sn-000001")
	kv.AddInDB(db, "subnet/sn-000001/vpc", "vp-admin")
	kv.AddInDB(db, "subnet/sn-000001/interface_ip", "10.1.1.1")
	kv.AddInDB(db, "subnet/sn-000001/dhcp/10.1.1.2", "00:22:33:00:01:02")
	kv.AddInDB(db, "vm/vm-1/ip", "10.1.1.2")
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

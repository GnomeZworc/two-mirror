package dispatcher

import (
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

// --- StartVMCommand.Prepare : écriture des disques en DB ---

func TestStartVMCommand_Prepare_SingleDisk(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")

	cmd := StartVMCommand{
		Name:  "vm-1",
		NICs:  []VMNIC{{Subnet: "sn-1", IP: "10.0.0.5", Primary: true}},
		Disks: []VMDisk{{Path: "/data/root.qcow2", Dev: "sda"}},
	}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}

	path, err := kv.GetFromDB(db, "vm/vm-1/disk/sda")
	if err != nil {
		t.Fatalf("clé disk/sda absente en DB : %v", err)
	}
	if path != "/data/root.qcow2" {
		t.Errorf("path attendu /data/root.qcow2, obtenu %q", path)
	}
}

func TestStartVMCommand_Prepare_MultiDisk(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")

	cmd := StartVMCommand{
		Name: "vm-2",
		NICs: []VMNIC{{Subnet: "sn-1", IP: "10.0.0.6", Primary: true}},
		Disks: []VMDisk{
			{Path: "/data/root.qcow2", Dev: "sda"},
			{Path: "/data/data.qcow2", Dev: "sdb"},
		},
	}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}

	for dev, want := range map[string]string{
		"sda": "/data/root.qcow2",
		"sdb": "/data/data.qcow2",
	} {
		got, err := kv.GetFromDB(db, "vm/vm-2/disk/"+dev)
		if err != nil {
			t.Fatalf("clé disk/%s absente en DB : %v", dev, err)
		}
		if got != want {
			t.Errorf("disk/%s : attendu %q, obtenu %q", dev, want, got)
		}
	}
}

func TestStartVMCommand_Prepare_SlotGap(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")

	// sdb absent au boot — slot réservé pour hotplug
	cmd := StartVMCommand{
		Name: "vm-3",
		NICs: []VMNIC{{Subnet: "sn-1", IP: "10.0.0.7", Primary: true}},
		Disks: []VMDisk{
			{Path: "/data/root.qcow2", Dev: "sda"},
			{Path: "/data/extra.qcow2", Dev: "sdc"},
		},
	}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}

	if _, err := kv.GetFromDB(db, "vm/vm-3/disk/sda"); err != nil {
		t.Fatalf("disk/sda absent : %v", err)
	}
	if _, err := kv.GetFromDB(db, "vm/vm-3/disk/sdc"); err != nil {
		t.Fatalf("disk/sdc absent : %v", err)
	}
	if _, err := kv.GetFromDB(db, "vm/vm-3/disk/sdb"); err == nil {
		t.Error("disk/sdb ne devrait pas exister en DB")
	}
}

func TestStartVMCommand_Prepare_NoVolumePath(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")

	cmd := StartVMCommand{
		Name:  "vm-4",
		NICs:  []VMNIC{{Subnet: "sn-1", IP: "10.0.0.8", Primary: true}},
		Disks: []VMDisk{{Path: "/data/root.qcow2", Dev: "sda"}},
	}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}

	if _, err := kv.GetFromDB(db, "vm/vm-4/volume_path"); err == nil {
		t.Error("volume_path ne devrait plus être écrit en DB")
	}
}

// --- StopVMCommand.Prepare ---

func TestStopVMCommand_Prepare_Success(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vm/vm-run/state", "running")
	cmd := StopVMCommand{Name: "vm-run"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	s, _ := kv.GetFromDB(db, "vm/vm-run/state")
	if s != "deleting" {
		t.Errorf("state attendu deleting, obtenu %q", s)
	}
}

func TestStopVMCommand_Prepare_RefusedWhileCreating(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vm/vm-wip/state", "creating")
	cmd := StopVMCommand{Name: "vm-wip"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait refuser l'arrêt d'une VM en creating")
	}
	s, _ := kv.GetFromDB(db, "vm/vm-wip/state")
	if s != "creating" {
		t.Errorf("l'état ne devrait pas changer, obtenu %q", s)
	}
}

func TestStopVMCommand_Prepare_AllowedFromError(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vm/vm-ko/state", "error")
	cmd := StopVMCommand{Name: "vm-ko"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare devrait accepter une VM en error : %v", err)
	}
}

func TestStopVMCommand_Prepare_NotFound(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := StopVMCommand{Name: "vm-inexistante"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait échouer si la VM n'existe pas")
	}
}

func TestStartVMCommand_Prepare_Duplicate(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vm/vm-exist/state", "running")

	cmd := StartVMCommand{
		Name:  "vm-exist",
		NICs:  []VMNIC{{Subnet: "sn-1", IP: "10.0.0.9", Primary: true}},
		Disks: []VMDisk{{Path: "/data/root.qcow2", Dev: "sda"}},
	}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait échouer si la VM existe déjà")
	}
}

// --- StartVMCommand.Prepare : documents cloud-init ---

func prepareWithDocuments(t *testing.T, docs map[string]string) *badger.DB {
	t.Helper()
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")

	cmd := StartVMCommand{
		Name:      "vm-doc",
		NICs:      []VMNIC{{Subnet: "sn-1", IP: "10.0.0.5", Primary: true}},
		Disks:     []VMDisk{{Path: "/data/root.qcow2", Dev: "vda"}},
		Documents: docs,
	}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	return db
}

func TestStartVMCommand_Prepare_StoresDocuments(t *testing.T) {
	userData := "#cloud-config\npackages:\n  - nginx\n"
	db := prepareWithDocuments(t, map[string]string{"user-data": userData})

	got, err := kv.GetFromDB(db, "vm/vm-doc/metadata/user-data")
	if err != nil {
		t.Fatalf("clé metadata/user-data absente : %v", err)
	}
	if got != userData {
		t.Errorf("user-data attendu %q, obtenu %q", userData, got)
	}
}

func TestStartVMCommand_Prepare_NoDocumentsWritesNoKey(t *testing.T) {
	db := prepareWithDocuments(t, nil)

	entries, err := kv.ListByPrefix(db, "vm/vm-doc/metadata/")
	if err != nil {
		t.Fatalf("ListByPrefix : %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("aucun document fourni, aucune clé ne doit être écrite : %v", entries)
	}
}

func TestStartVMCommand_Prepare_EmptyDocumentIsStored(t *testing.T) {
	db := prepareWithDocuments(t, map[string]string{"vendor-data": ""})

	entries, err := kv.ListByPrefix(db, "vm/vm-doc/metadata/")
	if err != nil {
		t.Fatalf("ListByPrefix : %v", err)
	}
	if _, ok := entries["vm/vm-doc/metadata/vendor-data"]; !ok {
		t.Error("un document explicitement vide doit être stocké : c'est une demande de ne rien servir, pas une absence de demande")
	}
}

func TestStartVMCommand_Prepare_AllDocumentKinds(t *testing.T) {
	docs := map[string]string{
		"user-data":      "u",
		"meta-data":      "m",
		"network-config": "n",
		"vendor-data":    "v",
	}
	db := prepareWithDocuments(t, docs)

	for doc, want := range docs {
		got, err := kv.GetFromDB(db, "vm/vm-doc/metadata/"+doc)
		if err != nil {
			t.Errorf("clé metadata/%s absente : %v", doc, err)
			continue
		}
		if got != want {
			t.Errorf("%s attendu %q, obtenu %q", doc, want, got)
		}
	}
}

func TestDeleteInDB_RemovesMetadataDocuments(t *testing.T) {
	db := prepareWithDocuments(t, map[string]string{"user-data": "u", "vendor-data": "v"})

	if err := kv.DeleteInDB(db, "vm/vm-doc"); err != nil {
		t.Fatalf("DeleteInDB : %v", err)
	}

	entries, err := kv.ListByPrefix(db, "vm/vm-doc/")
	if err != nil {
		t.Fatalf("ListByPrefix : %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("la suppression de la VM doit emporter ses documents : %v", entries)
	}
}

package dispatcher

import (
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
)

// --- StartVMCommand.Prepare : écriture des disques en DB ---

func TestStartVMCommand_Prepare_SingleDisk(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")

	cmd := StartVMCommand{
		Name:   "vm-1",
		Subnet: "sn-1",
		IP:     "10.0.0.5",
		Disks:  []VMDisk{{Path: "/data/root.qcow2", Dev: "sda"}},
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
		Name:   "vm-2",
		Subnet: "sn-1",
		IP:     "10.0.0.6",
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
		Name:   "vm-3",
		Subnet: "sn-1",
		IP:     "10.0.0.7",
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
		Name:   "vm-4",
		Subnet: "sn-1",
		IP:     "10.0.0.8",
		Disks:  []VMDisk{{Path: "/data/root.qcow2", Dev: "sda"}},
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
		Name:   "vm-exist",
		Subnet: "sn-1",
		IP:     "10.0.0.9",
		Disks:  []VMDisk{{Path: "/data/root.qcow2", Dev: "sda"}},
	}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait échouer si la VM existe déjà")
	}
}

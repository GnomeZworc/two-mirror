package agentapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
)

// --- vmFromDB ---

func TestVmFromDB_SingleDisk(t *testing.T) {
	entries := map[string]string{
		"vm/vm-1/state":         "running",
		"vm/vm-1/subnet":        "sn-1",
		"vm/vm-1/ip":            "10.0.0.5",
		"vm/vm-1/metadata_port": "1234",
		"vm/vm-1/memory":        "512",
		"vm/vm-1/cpus":          "1",
		"vm/vm-1/disk/sda":      "/data/root.qcow2",
	}
	vm, err := vmFromDB("vm-1", entries)
	if err != nil {
		t.Fatalf("vmFromDB a échoué : %v", err)
	}
	if len(vm.Storage) != 1 {
		t.Fatalf("attendu 1 disque, obtenu %d", len(vm.Storage))
	}
	if vm.Storage[0].Dev != "sda" || vm.Storage[0].Path != "/data/root.qcow2" {
		t.Errorf("disque inattendu : %+v", vm.Storage[0])
	}
}

func TestVmFromDB_MultiDisk(t *testing.T) {
	entries := map[string]string{
		"vm/vm-2/state":         "running",
		"vm/vm-2/subnet":        "sn-1",
		"vm/vm-2/ip":            "10.0.0.6",
		"vm/vm-2/metadata_port": "1235",
		"vm/vm-2/memory":        "1024",
		"vm/vm-2/cpus":          "2",
		"vm/vm-2/disk/sda":      "/data/root.qcow2",
		"vm/vm-2/disk/sdb":      "/data/data.qcow2",
	}
	vm, err := vmFromDB("vm-2", entries)
	if err != nil {
		t.Fatalf("vmFromDB a échoué : %v", err)
	}
	if len(vm.Storage) != 2 {
		t.Fatalf("attendu 2 disques, obtenu %d", len(vm.Storage))
	}
	sort.Slice(vm.Storage, func(i, j int) bool { return vm.Storage[i].Dev < vm.Storage[j].Dev })
	if vm.Storage[0].Dev != "sda" || vm.Storage[1].Dev != "sdb" {
		t.Errorf("devs attendus [sda sdb], obtenus [%s %s]", vm.Storage[0].Dev, vm.Storage[1].Dev)
	}
}

func TestVmFromDB_SlotGap(t *testing.T) {
	// sdb absent — sda et sdc seulement
	entries := map[string]string{
		"vm/vm-3/state":         "running",
		"vm/vm-3/subnet":        "sn-1",
		"vm/vm-3/ip":            "10.0.0.7",
		"vm/vm-3/metadata_port": "1236",
		"vm/vm-3/memory":        "512",
		"vm/vm-3/cpus":          "1",
		"vm/vm-3/disk/sda":      "/data/root.qcow2",
		"vm/vm-3/disk/sdc":      "/data/extra.qcow2",
	}
	vm, err := vmFromDB("vm-3", entries)
	if err != nil {
		t.Fatalf("vmFromDB a échoué : %v", err)
	}
	if len(vm.Storage) != 2 {
		t.Fatalf("attendu 2 disques, obtenu %d", len(vm.Storage))
	}
	devs := map[string]bool{}
	for _, s := range vm.Storage {
		devs[s.Dev] = true
	}
	if !devs["sda"] || !devs["sdc"] {
		t.Errorf("attendu sda et sdc, obtenus %v", devs)
	}
	if devs["sdb"] {
		t.Error("sdb ne devrait pas apparaître")
	}
}

// --- POST /vms ---

func TestStartVM_MultiDisk(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")

	body, _ := json.Marshal(VMCreateRequest{
		Name: "vm-10",
		Interfaces: []VMInterface{
			{Subnet: "sn-1", IP: "10.0.0.10", Primary: true},
		},
		Storage: []VMStorage{
			{Path: "/data/root.qcow2", Dev: "sda"},
			{Path: "/data/data.qcow2", Dev: "sdb"},
		},
		Memory: 1024,
		CPUs:   2,
	})

	w := httptest.NewRecorder()
	s.VmsHandler(w, httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}

	for _, dev := range []string{"sda", "sdb"} {
		if _, err := kv.GetFromDB(db, "vm/vm-10/disk/"+dev); err != nil {
			t.Errorf("disk/%s absent en DB après création", dev)
		}
	}
	if _, err := kv.GetFromDB(db, "vm/vm-10/volume_path"); err == nil {
		t.Error("volume_path ne devrait plus exister en DB")
	}
}

func TestStartVM_StorageReturnedInResponse(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")

	body, _ := json.Marshal(VMCreateRequest{
		Name: "vm-11",
		Interfaces: []VMInterface{
			{Subnet: "sn-1", IP: "10.0.0.11", Primary: true},
		},
		Storage: []VMStorage{
			{Path: "/data/root.qcow2", Dev: "sda"},
		},
	})

	w := httptest.NewRecorder()
	s.VmsHandler(w, httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d", w.Code)
	}

	var vm VM
	json.NewDecoder(w.Body).Decode(&vm)
	if len(vm.Storage) != 1 {
		t.Fatalf("attendu 1 disque dans la réponse, obtenu %d", len(vm.Storage))
	}
	if vm.Storage[0].Dev != "sda" || vm.Storage[0].Path != "/data/root.qcow2" {
		t.Errorf("disque inattendu dans la réponse : %+v", vm.Storage[0])
	}
}

package agentapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
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

// --- POST /vms : objet metadata ---

func postVM(t *testing.T, name string, meta VMMetadata) (*httptest.ResponseRecorder, *badger.DB) {
	t.Helper()
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")

	body, _ := json.Marshal(VMCreateRequest{
		Name:       name,
		Interfaces: []VMInterface{{Subnet: "sn-1", IP: "10.0.0.20", Primary: true}},
		Storage:    []VMStorage{{Path: "/data/root.qcow2", Dev: "vda"}},
		Memory:     1024,
		CPUs:       2,
		Metadata:   meta,
	})

	w := httptest.NewRecorder()
	s.VmsHandler(w, httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body)))
	return w, db
}

func TestStartVM_UserDataIsDecodedFromBase64(t *testing.T) {
	plain := "#cloud-config\npackages:\n  - nginx\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(plain))

	w, db := postVM(t, "vm-md1", VMMetadata{UserData: encoded})
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}

	got, err := kv.GetFromDB(db, "vm/vm-md1/metadata/user-data")
	if err != nil {
		t.Fatalf("user-data absent en DB : %v", err)
	}
	if got != plain {
		t.Errorf("user-data décodé attendu %q, obtenu %q", plain, got)
	}
}

func TestStartVM_InvalidBase64IsRejected(t *testing.T) {
	w, db := postVM(t, "vm-md2", VMMetadata{UserData: "ceci n'est pas du base64 !!"})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("un base64 invalide doit être refusé en 400, obtenu %d : %s", w.Code, w.Body.String())
	}
	if _, err := kv.GetFromDB(db, "vm/vm-md2/state"); err == nil {
		t.Error("aucune VM ne doit être créée quand la requête est refusée")
	}
}

func TestStartVM_InvalidBase64ErrorNamesTheField(t *testing.T) {
	w, _ := postVM(t, "vm-md3", VMMetadata{UserData: "@@@"})

	if !strings.Contains(w.Body.String(), "user_data") {
		t.Errorf("le message doit nommer le champ fautif : %s", w.Body.String())
	}
}

func TestStartVM_NoUserDataWritesNoDocument(t *testing.T) {
	w, db := postVM(t, "vm-md4", VMMetadata{SSHKey: "ssh-ed25519 AAAA user@host"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}

	entries, err := kv.ListByPrefix(db, "vm/vm-md4/metadata/")
	if err != nil {
		t.Fatalf("ListByPrefix : %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("sans user_data, aucun document ne doit être stocké : %v", entries)
	}
}

func TestStartVM_PasswordAndSSHKeyComeFromMetadata(t *testing.T) {
	w, db := postVM(t, "vm-md5", VMMetadata{
		Password: "$6$rounds$hash",
		SSHKey:   "ssh-ed25519 AAAA user@host",
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}

	if got, _ := kv.GetFromDB(db, "vm/vm-md5/password"); got != "$6$rounds$hash" {
		t.Errorf("password attendu depuis metadata, obtenu %q", got)
	}
	if got, _ := kv.GetFromDB(db, "vm/vm-md5/sshkey"); got != "ssh-ed25519 AAAA user@host" {
		t.Errorf("sshkey attendu depuis metadata, obtenu %q", got)
	}
}

func TestStartVM_EmptyBase64MeansNoDocument(t *testing.T) {
	w, db := postVM(t, "vm-md6", VMMetadata{UserData: base64.StdEncoding.EncodeToString([]byte(""))})
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}

	entries, _ := kv.ListByPrefix(db, "vm/vm-md6/metadata/")
	if len(entries) != 0 {
		t.Errorf("un base64 vide est indiscernable d'un champ absent : %v", entries)
	}
}

// --- interfaces multiples ---

func TestVmFromDB_MultipleInterfacesSortedByIndex(t *testing.T) {
	vm, err := vmFromDB("vm-multi", map[string]string{
		"vm/vm-multi/state":         "running",
		"vm/vm-multi/nic/1/subnet":  "sn-2",
		"vm/vm-multi/nic/1/ip":      "10.2.0.5",
		"vm/vm-multi/nic/0/subnet":  "sn-1",
		"vm/vm-multi/nic/0/ip":      "10.1.0.5",
		"vm/vm-multi/nic/0/primary": "true",
		"vm/vm-multi/disk/vda":      "/data/root.qcow2",
	})
	if err != nil {
		t.Fatalf("vmFromDB : %v", err)
	}
	if len(vm.Interfaces) != 2 {
		t.Fatalf("2 interfaces attendues, obtenu %d : %+v", len(vm.Interfaces), vm.Interfaces)
	}
	if vm.Interfaces[0].Subnet != "sn-1" || !vm.Interfaces[0].Primary {
		t.Errorf("la première doit être l'index 0, primaire : %+v", vm.Interfaces[0])
	}
	if vm.Interfaces[1].Subnet != "sn-2" || vm.Interfaces[1].Primary {
		t.Errorf("la seconde doit être l'index 1, non primaire : %+v", vm.Interfaces[1])
	}
}

func TestStartVM_StoresAllInterfaces(t *testing.T) {
	s, db := newTestServer(t)
	for _, sn := range []string{"sn-1", "sn-2"} {
		kv.AddInDB(db, "subnet/"+sn+"/state", "running")
		kv.AddInDB(db, "subnet/"+sn+"/vpc", "vpc-1")
	}

	body, _ := json.Marshal(VMCreateRequest{
		Name: "vm-multi",
		Interfaces: []VMInterface{
			{Subnet: "sn-1", IP: "10.1.0.5", Primary: true},
			{Subnet: "sn-2", IP: "10.2.0.5"},
		},
		Storage: []VMStorage{{Path: "/data/root.qcow2", Dev: "vda"}},
	})

	w := httptest.NewRecorder()
	s.VmsHandler(w, httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}

	if got, _ := kv.GetFromDB(db, "vm/vm-multi/nic/1/subnet"); got != "sn-2" {
		t.Errorf("seconde interface non stockée : %q", got)
	}
	if got, _ := kv.GetFromDB(db, "vm/vm-multi/nic/0/primary"); got != "true" {
		t.Errorf("primaire non marquée : %q", got)
	}
	if _, err := kv.GetFromDB(db, "vm/vm-multi/nic/1/primary"); err == nil {
		t.Error("une interface non primaire ne doit pas porter la clé primary")
	}
}

func TestStartVM_RejectsZeroOrTwoPrimaries(t *testing.T) {
	cases := map[string][]VMInterface{
		"aucune primaire": {{Subnet: "sn-1", IP: "10.1.0.5"}},
		"deux primaires": {
			{Subnet: "sn-1", IP: "10.1.0.5", Primary: true},
			{Subnet: "sn-2", IP: "10.2.0.5", Primary: true},
		},
	}
	for label, ifaces := range cases {
		s, db := newTestServer(t)
		for _, sn := range []string{"sn-1", "sn-2"} {
			kv.AddInDB(db, "subnet/"+sn+"/state", "running")
			kv.AddInDB(db, "subnet/"+sn+"/vpc", "vpc-1")
		}
		body, _ := json.Marshal(VMCreateRequest{
			Name:       "vm-bad",
			Interfaces: ifaces,
			Storage:    []VMStorage{{Path: "/data/root.qcow2", Dev: "vda"}},
		})
		w := httptest.NewRecorder()
		s.VmsHandler(w, httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body)))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s : attendu 400, obtenu %d — %s", label, w.Code, w.Body.String())
		}
	}
}

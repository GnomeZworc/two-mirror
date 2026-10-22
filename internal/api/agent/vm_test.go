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
		"vm/vm-1/disk/xvda":     "/data/root.qcow2",
	}
	vm, err := vmFromDB("vm-1", entries)
	if err != nil {
		t.Fatalf("vmFromDB a échoué : %v", err)
	}
	if len(vm.Storage) != 1 {
		t.Fatalf("attendu 1 disque, obtenu %d", len(vm.Storage))
	}
	if vm.Storage[0].Dev != "xvda" || vm.Storage[0].Path != "/data/root.qcow2" {
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
		"vm/vm-2/disk/xvda":     "/data/root.qcow2",
		"vm/vm-2/disk/xvdb":     "/data/data.qcow2",
	}
	vm, err := vmFromDB("vm-2", entries)
	if err != nil {
		t.Fatalf("vmFromDB a échoué : %v", err)
	}
	if len(vm.Storage) != 2 {
		t.Fatalf("attendu 2 disques, obtenu %d", len(vm.Storage))
	}
	sort.Slice(vm.Storage, func(i, j int) bool { return vm.Storage[i].Dev < vm.Storage[j].Dev })
	if vm.Storage[0].Dev != "xvda" || vm.Storage[1].Dev != "xvdb" {
		t.Errorf("devs attendus [xvda xvdb], obtenus [%s %s]", vm.Storage[0].Dev, vm.Storage[1].Dev)
	}
}

func TestVmFromDB_SlotGap(t *testing.T) {
	// xvdb absent — xvda et xvdc seulement
	entries := map[string]string{
		"vm/vm-3/state":         "running",
		"vm/vm-3/subnet":        "sn-1",
		"vm/vm-3/ip":            "10.0.0.7",
		"vm/vm-3/metadata_port": "1236",
		"vm/vm-3/memory":        "512",
		"vm/vm-3/cpus":          "1",
		"vm/vm-3/disk/xvda":     "/data/root.qcow2",
		"vm/vm-3/disk/xvdc":     "/data/extra.qcow2",
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
	if !devs["xvda"] || !devs["xvdc"] {
		t.Errorf("attendu xvda et xvdc, obtenus %v", devs)
	}
	if devs["xvdb"] {
		t.Error("xvdb ne devrait pas apparaître")
	}
}

// --- POST /vms ---

func TestStartVM_MultiDisk(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")
	kv.AddInDB(db, "subnet/sn-1/cidr", "10.0.0.0/24")

	body, _ := json.Marshal(VMCreateRequest{
		Name: "vm-10",
		Interfaces: []VMInterface{
			{Subnet: "sn-1", IP: "10.0.0.10", MAC: "52:54:00:00:00:05", Primary: true},
		},
		Storage: []VMStorage{
			{Path: "/data/root.qcow2", Dev: "vda"},
			{Path: "/data/data.qcow2", Dev: "xvda"},
		},
		Memory: 1024,
		CPUs:   2,
	})

	w := httptest.NewRecorder()
	s.VmsHandler(w, httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}

	for _, dev := range []string{"vda", "xvda"} {
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
	kv.AddInDB(db, "subnet/sn-1/cidr", "10.0.0.0/24")

	body, _ := json.Marshal(VMCreateRequest{
		Name: "vm-11",
		Interfaces: []VMInterface{
			{Subnet: "sn-1", IP: "10.0.0.11", MAC: "52:54:00:00:00:05", Primary: true},
		},
		Storage: []VMStorage{
			{Path: "/data/root.qcow2", Dev: "xvda"},
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
	if vm.Storage[0].Dev != "xvda" || vm.Storage[0].Path != "/data/root.qcow2" {
		t.Errorf("disque inattendu dans la réponse : %+v", vm.Storage[0])
	}
}

func TestStartVM_RejectsInvalidDiskDev(t *testing.T) {
	for _, dev := range []string{"sda", "sdb", "vdb", "xvd", "xvdA", "xvdaa", ""} {
		s, db := newTestServer(t)
		kv.AddInDB(db, "subnet/sn-1/state", "running")
		kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")
		kv.AddInDB(db, "subnet/sn-1/cidr", "10.0.0.0/24")

		body, _ := json.Marshal(VMCreateRequest{
			Name:       "vm-12",
			Interfaces: []VMInterface{{Subnet: "sn-1", IP: "10.0.0.12", MAC: "52:54:00:00:00:05", Primary: true}},
			Storage:    []VMStorage{{Path: "/data/root.qcow2", Dev: "vda"}, {Path: "/data/data.qcow2", Dev: dev}},
		})

		w := httptest.NewRecorder()
		s.VmsHandler(w, httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body)))
		if w.Code != http.StatusBadRequest {
			t.Errorf("dev %q : attendu 400, obtenu %d", dev, w.Code)
		}
		if !strings.Contains(w.Body.String(), "storage 1: dev") {
			t.Errorf("dev %q : message inattendu %s", dev, w.Body.String())
		}
		if _, err := kv.GetFromDB(db, "vm/vm-12/state"); err == nil {
			t.Errorf("dev %q : la VM ne doit pas être enregistrée", dev)
		}
	}
}

// --- POST /vms : objet metadata ---

func postVM(t *testing.T, name string, meta VMMetadata) (*httptest.ResponseRecorder, *badger.DB) {
	t.Helper()
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")
	kv.AddInDB(db, "subnet/sn-1/cidr", "10.0.0.0/24")

	body, _ := json.Marshal(VMCreateRequest{
		Name:       name,
		Interfaces: []VMInterface{{Subnet: "sn-1", IP: "10.0.0.20", MAC: "52:54:00:00:00:05", Primary: true}},
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
		"vm/vm-multi/nic/1/mac":     "52:54:00:aa:bb:cc",
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
	if vm.Interfaces[1].MAC != "52:54:00:aa:bb:cc" {
		t.Errorf("la mac de la seconde doit être rendue : %+v", vm.Interfaces[1])
	}
}

func TestStartVM_StoresAllInterfaces(t *testing.T) {
	s, db := newTestServer(t)
	for sn, cidr := range map[string]string{"sn-1": "10.1.0.0/24", "sn-2": "10.2.0.0/24"} {
		kv.AddInDB(db, "subnet/"+sn+"/state", "running")
		kv.AddInDB(db, "subnet/"+sn+"/vpc", "vpc-1")
		kv.AddInDB(db, "subnet/"+sn+"/cidr", cidr)
	}

	body, _ := json.Marshal(VMCreateRequest{
		Name: "vm-multi",
		Interfaces: []VMInterface{
			{Subnet: "sn-1", IP: "10.1.0.5", MAC: "52:54:00:00:00:05", Primary: true},
			{Subnet: "sn-2", IP: "10.2.0.5", MAC: "52:54:00:aa:bb:cc"},
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
	if got, _ := kv.GetFromDB(db, "vm/vm-multi/nic/1/mac"); got != "52:54:00:aa:bb:cc" {
		t.Errorf("mac de la seconde interface non stockée telle quelle : %q", got)
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
		"aucune primaire": {{Subnet: "sn-1", IP: "10.1.0.5", MAC: "52:54:00:00:00:05"}},
		"deux primaires": {
			{Subnet: "sn-1", IP: "10.1.0.5", MAC: "52:54:00:00:00:05", Primary: true},
			{Subnet: "sn-2", IP: "10.2.0.5", MAC: "52:54:00:00:00:05", Primary: true},
		},
	}
	for label, ifaces := range cases {
		s, db := newTestServer(t)
		for sn, cidr := range map[string]string{"sn-1": "10.1.0.0/24", "sn-2": "10.2.0.0/24"} {
			kv.AddInDB(db, "subnet/"+sn+"/state", "running")
			kv.AddInDB(db, "subnet/"+sn+"/vpc", "vpc-1")
			kv.AddInDB(db, "subnet/"+sn+"/cidr", cidr)
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

func TestStartVM_RejectsAnInterfaceWithoutMAC(t *testing.T) {
	s, db := newTestServer(t)
	for sn, cidr := range map[string]string{"sn-1": "10.1.0.0/24", "sn-2": "10.2.0.0/24"} {
		kv.AddInDB(db, "subnet/"+sn+"/state", "running")
		kv.AddInDB(db, "subnet/"+sn+"/vpc", "vpc-1")
		kv.AddInDB(db, "subnet/"+sn+"/cidr", cidr)
	}
	body, _ := json.Marshal(VMCreateRequest{
		Name: "vm-nomac",
		Interfaces: []VMInterface{
			{Subnet: "sn-1", IP: "10.1.0.5", MAC: "52:54:00:00:00:05", Primary: true},
			{Subnet: "sn-2", IP: "10.2.0.5"},
		},
		Storage: []VMStorage{{Path: "/data/root.qcow2", Dev: "vda"}},
	})
	w := httptest.NewRecorder()
	s.VmsHandler(w, httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body)))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "interface 1: mac is required") {
		t.Errorf("attendu 400 nommant l'interface 1, obtenu %d — %s", w.Code, w.Body.String())
	}
	if _, err := kv.GetFromDB(db, "vm/vm-nomac/state"); err == nil {
		t.Error("une VM refusée ne doit laisser aucune trace en base")
	}
}

func TestStartVM_RejectsReservedOrForeignAddresses(t *testing.T) {
	cases := map[string]string{
		"passerelle anycast": "10.0.0.254",
		"relais":             "10.0.0.253",
		"adresse réseau":     "10.0.0.0",
		"broadcast":          "10.0.0.255",
		"hors cidr":          "10.0.1.5",
		"gateway explicite":  "10.0.0.1",
	}
	for label, ip := range cases {
		s, db := newTestServer(t)
		kv.AddInDB(db, "subnet/sn-1/state", "running")
		kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")
		kv.AddInDB(db, "subnet/sn-1/cidr", "10.0.0.0/24")
		kv.AddInDB(db, "subnet/sn-1/interface_ip", "10.0.0.254")
		kv.AddInDB(db, "subnet/sn-1/relay_ip", "10.0.0.253")
		kv.AddInDB(db, "subnet/sn-1/gateway", "10.0.0.1")
		body, _ := json.Marshal(VMCreateRequest{
			Name:       "vm-ip",
			Interfaces: []VMInterface{{Subnet: "sn-1", IP: ip, MAC: "52:54:00:00:00:05", Primary: true}},
			Storage:    []VMStorage{{Path: "/data/root.qcow2", Dev: "vda"}},
		})
		w := httptest.NewRecorder()
		s.VmsHandler(w, httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body)))
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s : attendu 422, obtenu %d — %s", label, w.Code, w.Body.String())
		}
		if _, err := kv.GetFromDB(db, "vm/vm-ip/state"); err == nil {
			t.Errorf("%s : la VM ne doit pas être créée", label)
		}
	}
}

func TestStartVM_AcceptsHostAddressBesideReservedOnes(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")
	kv.AddInDB(db, "subnet/sn-1/cidr", "10.0.0.0/24")
	kv.AddInDB(db, "subnet/sn-1/interface_ip", "10.0.0.254")
	kv.AddInDB(db, "subnet/sn-1/relay_ip", "10.0.0.253")
	body, _ := json.Marshal(VMCreateRequest{
		Name:       "vm-ip",
		Interfaces: []VMInterface{{Subnet: "sn-1", IP: "10.0.0.252", MAC: "52:54:00:00:00:05", Primary: true}},
		Storage:    []VMStorage{{Path: "/data/root.qcow2", Dev: "vda"}},
	})
	w := httptest.NewRecorder()
	s.VmsHandler(w, httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}
}

func TestStartVM_RejectsSubnetWithoutCIDR(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")
	body, _ := json.Marshal(VMCreateRequest{
		Name:       "vm-ip",
		Interfaces: []VMInterface{{Subnet: "sn-1", IP: "10.0.0.5", MAC: "52:54:00:00:00:05", Primary: true}},
		Storage:    []VMStorage{{Path: "/data/root.qcow2", Dev: "vda"}},
	})
	w := httptest.NewRecorder()
	s.VmsHandler(w, httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body)))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "has no cidr") {
		t.Errorf("attendu 422 « has no cidr », obtenu %d : %s", w.Code, w.Body.String())
	}
}

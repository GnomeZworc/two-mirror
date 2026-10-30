package agentapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

func seedRunningVM(t *testing.T, db *badger.DB) {
	t.Helper()
	for key, value := range map[string]string{
		"vpc/vp-t47/cidr":      "10.47.0.0/16",
		"subnet/sn-a/vpc":      "vp-t47",
		"subnet/sn-a/mode":     "vxlan",
		"vm/vm-1/state":        "running",
		"vm/vm-1/nic/0/name":   "nic-a",
		"vm/vm-1/nic/0/subnet": "sn-a",
		"vm/vm-1/nic/0/ip":     "10.47.1.10",
		"vm/vm-1/nic/1/name":   "nic-b",
		"vm/vm-1/nic/1/subnet": "sn-a",
		"vm/vm-1/nic/1/ip":     "10.47.1.11",
	} {
		kv.AddInDB(db, key, value)
	}
}

func seedFloatingIP(t *testing.T, db *badger.DB, name, vm, iface, st string) {
	t.Helper()
	kv.AddInDB(db, "floating-ip/"+name+"/state", st)
	kv.AddInDB(db, "floating-ip/"+name+"/vm", vm)
	kv.AddInDB(db, "floating-ip/"+name+"/interface", iface)
	kv.AddInDB(db, "floating-ip/"+name+"/ip", "240.0.0.47/24")
	kv.AddInDB(db, "floating-ip/"+name+"/gateway", "240.0.0.1")
}

func validFloatingIPRequest() FloatingIPCreateRequest {
	return FloatingIPCreateRequest{Name: "fip-1", VM: "vm-1", Interface: "nic-a", IP: "240.0.0.47/24", Gateway: "240.0.0.1"}
}

func postFloatingIP(t *testing.T, s *Server, req FloatingIPCreateRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.FloatingIPsHandler(w, httptest.NewRequest(http.MethodPost, "/floating-ips", bytes.NewReader(body)))
	return w
}

func TestPostFloatingIP_Accepted(t *testing.T) {
	s, db := newTestServer(t)
	seedRunningVM(t, db)
	w := postFloatingIP(t, s, validFloatingIPRequest())
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}
	var got FloatingIP
	json.NewDecoder(w.Body).Decode(&got)
	want := FloatingIP{Name: "fip-1", State: "creating", VM: "vm-1", Interface: "nic-a", IP: "240.0.0.47/24", Gateway: "240.0.0.1"}
	if got != want {
		t.Errorf("réponse %+v, attendu %+v", got, want)
	}
}

func TestPostFloatingIP_BadRequest(t *testing.T) {
	cases := []struct {
		label  string
		mutate func(*FloatingIPCreateRequest)
		msg    string
	}{
		{"nom absent", func(r *FloatingIPCreateRequest) { r.Name = "" }, "name is required"},
		{"vm absente", func(r *FloatingIPCreateRequest) { r.VM = "" }, "vm is required"},
		{"interface absente", func(r *FloatingIPCreateRequest) { r.Interface = "" }, "interface is required"},
		{"ip absente", func(r *FloatingIPCreateRequest) { r.IP = "" }, "ip is required"},
		{"gateway absente", func(r *FloatingIPCreateRequest) { r.Gateway = "" }, "gateway is required"},
		{"ip sans préfixe", func(r *FloatingIPCreateRequest) { r.IP = "240.0.0.47" }, "invalid ip"},
		{"gateway IPv6", func(r *FloatingIPCreateRequest) { r.Gateway = "2001:db8::1" }, "invalid gateway"},
	}
	for _, c := range cases {
		s, db := newTestServer(t)
		seedRunningVM(t, db)
		req := validFloatingIPRequest()
		c.mutate(&req)
		w := postFloatingIP(t, s, req)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), c.msg) {
			t.Errorf("%s : %d %s, 400 « %s » attendu", c.label, w.Code, w.Body.String(), c.msg)
		}
		if _, err := kv.GetFromDB(db, "floating-ip/fip-1/state"); err == nil {
			t.Errorf("%s : IP flottante écrite malgré le refus", c.label)
		}
	}
}

func TestPostFloatingIP_ConflictOnUnknownInterface(t *testing.T) {
	s, db := newTestServer(t)
	seedRunningVM(t, db)
	req := validFloatingIPRequest()
	req.Interface = "nic-z"
	w := postFloatingIP(t, s, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `has no interface \"nic-z\"`) {
		t.Errorf("%d %s, 409 attendu", w.Code, w.Body.String())
	}
}

func TestFloatingIPGetListAndDelete(t *testing.T) {
	s, db := newTestServer(t)
	seedFloatingIP(t, db, "fip-1", "vm-1", "nic-a", "running")
	want := FloatingIP{Name: "fip-1", State: "running", VM: "vm-1", Interface: "nic-a", IP: "240.0.0.47/24", Gateway: "240.0.0.1"}

	w := httptest.NewRecorder()
	s.FloatingIPsHandler(w, httptest.NewRequest(http.MethodGet, "/floating-ips", nil))
	var list []FloatingIP
	json.NewDecoder(w.Body).Decode(&list)
	if w.Code != http.StatusOK || len(list) != 1 || list[0] != want {
		t.Errorf("liste %d %+v", w.Code, list)
	}

	w = httptest.NewRecorder()
	s.FloatingIPByNameHandler(w, httptest.NewRequest(http.MethodGet, "/floating-ips/fip-1", nil))
	var got FloatingIP
	json.NewDecoder(w.Body).Decode(&got)
	if w.Code != http.StatusOK || got != want {
		t.Errorf("get %d %+v", w.Code, got)
	}

	w = httptest.NewRecorder()
	s.FloatingIPByNameHandler(w, httptest.NewRequest(http.MethodDelete, "/floating-ips/fip-x", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("suppression d'une IP flottante absente : %d, 404 attendu", w.Code)
	}
	w = httptest.NewRecorder()
	s.FloatingIPByNameHandler(w, httptest.NewRequest(http.MethodDelete, "/floating-ips/fip-1", nil))
	if w.Code != http.StatusAccepted {
		t.Errorf("suppression : %d, 202 attendu", w.Code)
	}
}

func TestGetVM_ShowsTheFloatingIPOfItsInterface(t *testing.T) {
	s, db := newTestServer(t)
	seedRunningVM(t, db)
	seedFloatingIP(t, db, "fip-1", "vm-1", "nic-b", "running")
	seedFloatingIP(t, db, "fip-9", "vm-10", "nic-a", "running")
	w := httptest.NewRecorder()
	s.VmByNameHandler(w, httptest.NewRequest(http.MethodGet, "/vms/vm-1", nil))
	raw := w.Body.String()
	var vm VM
	json.Unmarshal([]byte(raw), &vm)
	if len(vm.Interfaces) != 2 {
		t.Fatalf("interfaces %+v", vm.Interfaces)
	}
	if vm.Interfaces[0].FloatingIP != nil {
		t.Errorf("nic-a porte une IP flottante qui n'est pas la sienne : %+v", vm.Interfaces[0].FloatingIP)
	}
	want := VMFloatingIP{Name: "fip-1", IP: "240.0.0.47/24", State: "running"}
	if vm.Interfaces[1].FloatingIP == nil || *vm.Interfaces[1].FloatingIP != want {
		t.Errorf("nic-b : %+v, attendu %+v", vm.Interfaces[1].FloatingIP, want)
	}
	if strings.Count(raw, "floating_ip") != 1 {
		t.Errorf("floating_ip doit être absent du JSON d'une interface sans IP flottante : %s", raw)
	}
}

func TestDeleteVM_BlockedByAFloatingIP(t *testing.T) {
	s, db := newTestServer(t)
	seedRunningVM(t, db)
	seedFloatingIP(t, db, "fip-1", "vm-1", "nic-a", "running")
	w := httptest.NewRecorder()
	s.VmByNameHandler(w, httptest.NewRequest(http.MethodDelete, "/vms/vm-1", nil))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `floating ip \"fip-1\" must be deleted`) {
		t.Errorf("%d %s, 409 attendu", w.Code, w.Body.String())
	}
}

func TestStartVM_RefusesAFloatingIPInTheRequest(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")
	kv.AddInDB(db, "subnet/sn-1/cidr", "10.0.0.0/24")
	body, _ := json.Marshal(VMCreateRequest{
		Name:       "vm-f",
		Interfaces: []VMInterface{{Name: "nic-a", Subnet: "sn-1", IP: "10.0.0.10", MAC: "52:54:00:00:00:05", Primary: true, FloatingIP: &VMFloatingIP{Name: "fip-1"}}},
		Storage:    []VMStorage{{Path: "/data/root.qcow2", Dev: "vda"}},
	})
	w := httptest.NewRecorder()
	s.VmsHandler(w, httptest.NewRequest(http.MethodPost, "/vms", bytes.NewReader(body)))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "floating_ip is read-only") {
		t.Errorf("%d %s, 400 attendu", w.Code, w.Body.String())
	}
	if _, err := kv.GetFromDB(db, "vm/vm-f/state"); err == nil {
		t.Error("une VM refusée ne doit laisser aucune trace en base")
	}
}

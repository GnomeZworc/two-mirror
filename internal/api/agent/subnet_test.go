package agentapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
)

// --- SubnetsHandler ---

func TestListSubnets_Empty(t *testing.T) {
	s, _ := newTestServer(t)
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodGet, "/subnets", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("attendu 200, obtenu %d", w.Code)
	}
	var result []Subnet
	json.NewDecoder(w.Body).Decode(&result)
	if len(result) != 0 {
		t.Errorf("attendu liste vide, obtenu %v", result)
	}
}

func TestListSubnets_WithData(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")
	kv.AddInDB(db, "subnet/sn-2/state", "creating")
	kv.AddInDB(db, "subnet/sn-2/vpc", "vpc-1")
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodGet, "/subnets", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("attendu 200, obtenu %d", w.Code)
	}
	var result []Subnet
	json.NewDecoder(w.Body).Decode(&result)
	if len(result) != 2 {
		t.Errorf("attendu 2 subnets, obtenu %d", len(result))
	}
}

func TestListSubnets_InvalidMethod(t *testing.T) {
	s, _ := newTestServer(t)
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPut, "/subnets", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("attendu 405, obtenu %d", w.Code)
	}
}

func TestPostSubnet_Created(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	req := SubnetCreateRequest{
		Name:      "sn-new",
		VPC:       "vpc-1",
		IfaceType: "vms",
		CIDR:      "10.0.0.0/24",
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d: %s", w.Code, w.Body.String())
	}
	var result Subnet
	json.NewDecoder(w.Body).Decode(&result)
	if result.Name != "sn-new" {
		t.Errorf("name attendu sn-new, obtenu %q", result.Name)
	}
	if result.State != "creating" {
		t.Errorf("state attendu creating, obtenu %q", result.State)
	}
}

func TestPostSubnet_MissingFields(t *testing.T) {
	s, _ := newTestServer(t)
	body, _ := json.Marshal(SubnetCreateRequest{Name: "sn-1"}) // vpc, gateway_ip, cidr manquants
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("attendu 400, obtenu %d", w.Code)
	}
}

func TestPostSubnet_IfaceTypeOptional(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	req := SubnetCreateRequest{
		Name: "sn-opt",
		VPC:  "vpc-1",
		CIDR: "10.0.0.0/24",
		// IfaceType omis — doit utiliser default_interface
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d: %s", w.Code, w.Body.String())
	}
}

func TestPostSubnet_VPCNotFound(t *testing.T) {
	s, _ := newTestServer(t)
	req := SubnetCreateRequest{
		Name:      "sn-1",
		VPC:       "vpc-inexistant",
		IfaceType: "vms",
		CIDR:      "10.0.0.0/24",
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader(body)))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("attendu 422, obtenu %d", w.Code)
	}
}

func TestPostSubnet_Duplicate(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	kv.AddInDB(db, "subnet/sn-exist/state", "running")
	req := SubnetCreateRequest{
		Name:      "sn-exist",
		VPC:       "vpc-1",
		IfaceType: "vms",
		CIDR:      "10.0.0.0/24",
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader(body)))
	if w.Code != http.StatusConflict {
		t.Errorf("attendu 409, obtenu %d", w.Code)
	}
}

func TestPostSubnet_VPCDeleting(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-dying/state", "deleting")
	req := SubnetCreateRequest{
		Name:      "sn-1",
		VPC:       "vpc-dying",
		IfaceType: "vms",
		CIDR:      "10.0.0.0/24",
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader(body)))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("attendu 422, obtenu %d", w.Code)
	}
}

func TestPostSubnet_BridgeMode_Success(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	req := SubnetCreateRequest{
		Name:        "sn-br",
		VPC:         "vpc-1",
		Mode:        "bridge",
		IfaceType:   "vms",
		InterfaceIP: "10.0.0.1",
		CIDR:        "10.0.0.0/24",
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d: %s", w.Code, w.Body.String())
	}
	var result Subnet
	json.NewDecoder(w.Body).Decode(&result)
	if result.Mode != "bridge" {
		t.Errorf("mode attendu bridge, obtenu %q", result.Mode)
	}
	if result.VxlanID != 0 {
		t.Errorf("vxlan_id devrait être 0 en mode bridge, obtenu %d", result.VxlanID)
	}
}

func TestPostSubnet_UnknownMode(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	req := SubnetCreateRequest{
		Name:        "sn-1",
		VPC:         "vpc-1",
		Mode:        "vlan",
		InterfaceIP: "10.0.0.1",
		CIDR:        "10.0.0.0/24",
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader(body)))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("attendu 422, obtenu %d", w.Code)
	}
}

func TestPostSubnet_UnknownModeWithoutInterfaceIP(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	body, _ := json.Marshal(SubnetCreateRequest{Name: "sn-1", VPC: "vpc-1", Mode: "vlan", CIDR: "10.0.0.0/24"})
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader(body)))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "unknown subnet mode") {
		t.Errorf("attendu 422 mode inconnu, obtenu %d : %s", w.Code, w.Body.String())
	}
}

func TestPostSubnet_InvalidBody(t *testing.T) {
	s, _ := newTestServer(t)
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader([]byte("not json"))))
	if w.Code != http.StatusBadRequest {
		t.Errorf("attendu 400, obtenu %d", w.Code)
	}
}

// --- SubnetByNameHandler ---

func TestGetSubnet_Found(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")
	kv.AddInDB(db, "subnet/sn-1/cidr", "10.0.0.0/24")
	kv.AddInDB(db, "subnet/sn-1/interface_ip", "10.0.0.1")
	req := httptest.NewRequest(http.MethodGet, "/subnets/sn-1", nil)
	w := httptest.NewRecorder()
	s.SubnetByNameHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("attendu 200, obtenu %d", w.Code)
	}
	var result Subnet
	json.NewDecoder(w.Body).Decode(&result)
	if result.Name != "sn-1" || result.State != "running" {
		t.Errorf("résultat inattendu : %+v", result)
	}
	if result.VPC != "vpc-1" {
		t.Errorf("vpc attendu vpc-1, obtenu %q", result.VPC)
	}
}

func TestGetSubnet_NotFound(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/subnets/inexistant", nil)
	w := httptest.NewRecorder()
	s.SubnetByNameHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("attendu 404, obtenu %d", w.Code)
	}
}

func TestGetSubnet_EmptyName(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/subnets/", nil)
	w := httptest.NewRecorder()
	s.SubnetByNameHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("attendu 404, obtenu %d", w.Code)
	}
}

func TestDeleteSubnet_Success(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-del/state", "running")
	req := httptest.NewRequest(http.MethodDelete, "/subnets/sn-del", nil)
	w := httptest.NewRecorder()
	s.SubnetByNameHandler(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d: %s", w.Code, w.Body.String())
	}
	var result Subnet
	json.NewDecoder(w.Body).Decode(&result)
	if result.State != "deleting" {
		t.Errorf("state attendu deleting, obtenu %q", result.State)
	}
}

func TestDeleteSubnet_ConflictWhileCreating(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-wip/state", "creating")
	req := httptest.NewRequest(http.MethodDelete, "/subnets/sn-wip", nil)
	w := httptest.NewRecorder()
	s.SubnetByNameHandler(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("attendu 409, obtenu %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteSubnet_NotFound(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/subnets/inexistant", nil)
	w := httptest.NewRecorder()
	s.SubnetByNameHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("attendu 404, obtenu %d", w.Code)
	}
}

func TestSubnetByName_InvalidMethod(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	req := httptest.NewRequest(http.MethodPut, "/subnets/sn-1", nil)
	w := httptest.NewRecorder()
	s.SubnetByNameHandler(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("attendu 405, obtenu %d", w.Code)
	}
}

func TestCreateSubnet_GatewayRoundTrip(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")

	body, _ := json.Marshal(SubnetCreateRequest{
		Name: "sn-gw", VPC: "vpc-1", Mode: "public_ip",
		IfaceType: "vms", InterfaceIP: "10.0.0.1", CIDR: "10.0.0.0/24",
		DefaultRoute: true, Gateway: "203.0.113.1",
	})

	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}
	var created Subnet
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("réponse 202 illisible : %v", err)
	}
	if created.Gateway != "203.0.113.1" {
		t.Errorf("gateway absente de la réponse 202 : %+v", created)
	}

	if gw, _ := kv.GetFromDB(db, "subnet/sn-gw/gateway"); gw != "203.0.113.1" {
		t.Errorf("gateway attendue en DB, obtenu %q", gw)
	}

	w = httptest.NewRecorder()
	s.SubnetByNameHandler(w, httptest.NewRequest(http.MethodGet, "/subnets/sn-gw", nil))
	var got Subnet
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("réponse illisible : %v", err)
	}
	if got.Gateway != "203.0.113.1" {
		t.Errorf("gateway absente de la réponse GET : %+v", got)
	}

	w = httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodGet, "/subnets", nil))
	var list []Subnet
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("liste illisible : %v", err)
	}
	if len(list) != 1 || list[0].Gateway != "203.0.113.1" {
		t.Errorf("gateway absente de la liste : %+v", list)
	}
}

func TestSubnet_VxlanIDOnlyForVxlan(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	vx := postSubnetRequest(t, s, SubnetCreateRequest{
		Name: "sn-vx", VPC: "vpc-1", Mode: "vxlan", VxlanID: 65537, CIDR: "10.0.0.0/24",
	})
	if vx.Code != http.StatusAccepted {
		t.Fatalf("vxlan : attendu 202, obtenu %d : %s", vx.Code, vx.Body.String())
	}
	if !strings.Contains(vx.Body.String(), `"vxlan_id":65537`) {
		t.Errorf("vxlan_id absent de la réponse 202 vxlan : %s", vx.Body.String())
	}
	br := postSubnetRequest(t, s, SubnetCreateRequest{
		Name: "sn-br", VPC: "vpc-1", Mode: "bridge", VxlanID: 65538, InterfaceIP: "10.1.0.1", CIDR: "10.1.0.0/24",
	})
	if br.Code != http.StatusAccepted {
		t.Fatalf("bridge : attendu 202, obtenu %d : %s", br.Code, br.Body.String())
	}
	if strings.Contains(br.Body.String(), "vxlan_id") {
		t.Errorf("vxlan_id ne doit pas apparaître dans la réponse 202 bridge : %s", br.Body.String())
	}

	w := httptest.NewRecorder()
	s.SubnetByNameHandler(w, httptest.NewRequest(http.MethodGet, "/subnets/sn-br", nil))
	if strings.Contains(w.Body.String(), "vxlan_id") {
		t.Errorf("vxlan_id ne doit pas apparaître pour un subnet bridge : %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodGet, "/subnets", nil))
	var list []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("liste illisible : %v", err)
	}
	for _, sub := range list {
		_, has := sub["vxlan_id"]
		if want := sub["name"] == "sn-vx"; has != want {
			t.Errorf("%v : vxlan_id présent=%v, attendu %v", sub["name"], has, want)
		}
	}
}

func postSubnetRequest(t *testing.T, s *Server, req SubnetCreateRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader(body)))
	return w
}

func TestPostSubnet_VxlanRejectsInterfaceIP(t *testing.T) {
	for _, mode := range []string{"", "vxlan"} {
		s, db := newTestServer(t)
		kv.AddInDB(db, "vpc/vpc-1/state", "running")
		w := postSubnetRequest(t, s, SubnetCreateRequest{
			Name: "sn-1", VPC: "vpc-1", Mode: mode,
			InterfaceIP: "10.0.0.1", CIDR: "10.0.0.0/24",
		})
		if w.Code != http.StatusBadRequest {
			t.Errorf("mode %q : attendu 400, obtenu %d : %s", mode, w.Code, w.Body.String())
		}
		if _, err := kv.GetFromDB(db, "subnet/sn-1/state"); err == nil {
			t.Errorf("mode %q : aucun subnet ne doit être créé", mode)
		}
	}
}

func TestPostSubnet_BridgeRequiresInterfaceIP(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	w := postSubnetRequest(t, s, SubnetCreateRequest{
		Name: "sn-br", VPC: "vpc-1", Mode: "bridge", CIDR: "10.0.0.0/24",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("attendu 400, obtenu %d : %s", w.Code, w.Body.String())
	}
}

func TestPostSubnet_VxlanReturnsComputedAddresses(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	w := postSubnetRequest(t, s, SubnetCreateRequest{
		Name: "sn-vx", VPC: "vpc-1", Mode: "vxlan", CIDR: "10.0.0.0/24",
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}
	var got Subnet
	json.NewDecoder(w.Body).Decode(&got)
	if got.InterfaceIP != "10.0.0.254" {
		t.Errorf("interface_ip attendue 10.0.0.254, obtenu %q", got.InterfaceIP)
	}
	if got.RelayIP != "10.0.0.253" {
		t.Errorf("relay_ip attendu 10.0.0.253, obtenu %q", got.RelayIP)
	}
}

func TestGetSubnet_RelayIPOnlyForVxlan(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	if w := postSubnetRequest(t, s, SubnetCreateRequest{
		Name: "sn-vx", VPC: "vpc-1", CIDR: "10.0.0.0/24",
	}); w.Code != http.StatusAccepted {
		t.Fatalf("vxlan : attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}
	if w := postSubnetRequest(t, s, SubnetCreateRequest{
		Name: "sn-br", VPC: "vpc-1", Mode: "bridge", InterfaceIP: "10.1.0.1", CIDR: "10.1.0.0/24",
	}); w.Code != http.StatusAccepted {
		t.Fatalf("bridge : attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}

	w := httptest.NewRecorder()
	s.SubnetByNameHandler(w, httptest.NewRequest(http.MethodGet, "/subnets/sn-vx", nil))
	var vx Subnet
	json.NewDecoder(w.Body).Decode(&vx)
	if vx.RelayIP != "10.0.0.253" {
		t.Errorf("relay_ip attendu 10.0.0.253, obtenu %q", vx.RelayIP)
	}

	w = httptest.NewRecorder()
	s.SubnetByNameHandler(w, httptest.NewRequest(http.MethodGet, "/subnets/sn-br", nil))
	if strings.Contains(w.Body.String(), "relay_ip") {
		t.Errorf("relay_ip ne doit pas apparaître pour un subnet bridge : %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodGet, "/subnets", nil))
	var list []Subnet
	json.NewDecoder(w.Body).Decode(&list)
	relays := map[string]string{}
	for _, sub := range list {
		relays[sub.Name] = sub.RelayIP
	}
	if relays["sn-vx"] != "10.0.0.253" || relays["sn-br"] != "" {
		t.Errorf("relay_ip inattendus dans la liste : %v", relays)
	}
}

package agentapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	kv.AddInDB(db, "subnet/sn-1/state", "created")
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
	kv.AddInDB(db, "vpc/vpc-1/state", "created")
	req := SubnetCreateRequest{
		Name:      "sn-new",
		VPC:       "vpc-1",
		IfaceType: "vms",
		GatewayIP: "10.0.0.1",
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
	kv.AddInDB(db, "vpc/vpc-1/state", "created")
	req := SubnetCreateRequest{
		Name:      "sn-opt",
		VPC:       "vpc-1",
		GatewayIP: "10.0.0.1",
		CIDR:      "10.0.0.0/24",
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
		GatewayIP: "10.0.0.1",
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
	kv.AddInDB(db, "vpc/vpc-1/state", "created")
	kv.AddInDB(db, "subnet/sn-exist/state", "created")
	req := SubnetCreateRequest{
		Name:      "sn-exist",
		VPC:       "vpc-1",
		IfaceType: "vms",
		GatewayIP: "10.0.0.1",
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
		GatewayIP: "10.0.0.1",
		CIDR:      "10.0.0.0/24",
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.SubnetsHandler(w, httptest.NewRequest(http.MethodPost, "/subnets", bytes.NewReader(body)))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("attendu 422, obtenu %d", w.Code)
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
	kv.AddInDB(db, "subnet/sn-1/state", "created")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-1")
	kv.AddInDB(db, "subnet/sn-1/cidr", "10.0.0.0/24")
	kv.AddInDB(db, "subnet/sn-1/gateway_ip", "10.0.0.1")
	req := httptest.NewRequest(http.MethodGet, "/subnets/sn-1", nil)
	w := httptest.NewRecorder()
	s.SubnetByNameHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("attendu 200, obtenu %d", w.Code)
	}
	var result Subnet
	json.NewDecoder(w.Body).Decode(&result)
	if result.Name != "sn-1" || result.State != "created" {
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
	kv.AddInDB(db, "subnet/sn-del/state", "created")
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
	kv.AddInDB(db, "subnet/sn-1/state", "created")
	req := httptest.NewRequest(http.MethodPut, "/subnets/sn-1", nil)
	w := httptest.NewRecorder()
	s.SubnetByNameHandler(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("attendu 405, obtenu %d", w.Code)
	}
}

package agentapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

// --- VpcsHandler ---

func TestListVpcs_Empty(t *testing.T) {
	s, _ := newTestServer(t)
	w := httptest.NewRecorder()
	s.VpcsHandler(w, httptest.NewRequest(http.MethodGet, "/vpcs", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("attendu 200, obtenu %d", w.Code)
	}
	var result []VPC
	json.NewDecoder(w.Body).Decode(&result)
	if len(result) != 0 {
		t.Errorf("attendu liste vide, obtenu %v", result)
	}
}

func TestListVpcs_WithData(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/v1/state", "running")
	kv.AddInDB(db, "vpc/v2/state", "creating")
	w := httptest.NewRecorder()
	s.VpcsHandler(w, httptest.NewRequest(http.MethodGet, "/vpcs", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("attendu 200, obtenu %d", w.Code)
	}
	var result []VPC
	json.NewDecoder(w.Body).Decode(&result)
	if len(result) != 2 {
		t.Errorf("attendu 2 VPCs, obtenu %d", len(result))
	}
}

func TestListVpcs_InvalidMethod(t *testing.T) {
	s, _ := newTestServer(t)
	w := httptest.NewRecorder()
	s.VpcsHandler(w, httptest.NewRequest(http.MethodPut, "/vpcs", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("attendu 405, obtenu %d", w.Code)
	}
}

func TestPostVpc_Created(t *testing.T) {
	s, _ := newTestServer(t)
	body, _ := json.Marshal(VPCCreateRequest{Name: "vpc-new", CIDR: "10.0.0.0/16"})
	w := httptest.NewRecorder()
	s.VpcsHandler(w, httptest.NewRequest(http.MethodPost, "/vpcs", bytes.NewReader(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d: %s", w.Code, w.Body.String())
	}
	var result VPC
	json.NewDecoder(w.Body).Decode(&result)
	if result.Name != "vpc-new" {
		t.Errorf("name attendu vpc-new, obtenu %q", result.Name)
	}
	if result.State != "creating" {
		t.Errorf("state attendu creating, obtenu %q", result.State)
	}
	if result.CIDR != "10.0.0.0/16" {
		t.Errorf("cidr attendu 10.0.0.0/16, obtenu %q", result.CIDR)
	}
}

func TestPostVpc_MissingName(t *testing.T) {
	s, _ := newTestServer(t)
	body, _ := json.Marshal(VPCCreateRequest{CIDR: "10.0.0.0/16"})
	w := httptest.NewRecorder()
	s.VpcsHandler(w, httptest.NewRequest(http.MethodPost, "/vpcs", bytes.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("attendu 400, obtenu %d", w.Code)
	}
}

func TestPostVpc_MissingCIDR(t *testing.T) {
	s, _ := newTestServer(t)
	body, _ := json.Marshal(VPCCreateRequest{Name: "vpc-new"})
	w := httptest.NewRecorder()
	s.VpcsHandler(w, httptest.NewRequest(http.MethodPost, "/vpcs", bytes.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("attendu 400, obtenu %d", w.Code)
	}
}

func TestPostVpc_InvalidCIDR(t *testing.T) {
	s, _ := newTestServer(t)
	body, _ := json.Marshal(VPCCreateRequest{Name: "vpc-new", CIDR: "not-a-cidr"})
	w := httptest.NewRecorder()
	s.VpcsHandler(w, httptest.NewRequest(http.MethodPost, "/vpcs", bytes.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("attendu 400, obtenu %d", w.Code)
	}
}

func TestPostVpc_Duplicate(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-exist/state", "running")
	body, _ := json.Marshal(VPCCreateRequest{Name: "vpc-exist", CIDR: "10.0.0.0/16"})
	w := httptest.NewRecorder()
	s.VpcsHandler(w, httptest.NewRequest(http.MethodPost, "/vpcs", bytes.NewReader(body)))
	if w.Code != http.StatusConflict {
		t.Errorf("attendu 409, obtenu %d", w.Code)
	}
}

func TestPostVpc_InvalidBody(t *testing.T) {
	s, _ := newTestServer(t)
	w := httptest.NewRecorder()
	s.VpcsHandler(w, httptest.NewRequest(http.MethodPost, "/vpcs", bytes.NewReader([]byte("not json"))))
	if w.Code != http.StatusBadRequest {
		t.Errorf("attendu 400, obtenu %d", w.Code)
	}
}

// --- VpcByNameHandler ---

func TestGetVpc_Found(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	req := httptest.NewRequest(http.MethodGet, "/vpcs/vpc-1", nil)
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("attendu 200, obtenu %d", w.Code)
	}
	var result VPC
	json.NewDecoder(w.Body).Decode(&result)
	if result.Name != "vpc-1" || result.State != "running" {
		t.Errorf("résultat inattendu : %+v", result)
	}
}

func TestGetVpc_NotFound(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/vpcs/inexistant", nil)
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("attendu 404, obtenu %d", w.Code)
	}
}

func TestGetVpc_EmptyName(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/vpcs/", nil)
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("attendu 404, obtenu %d", w.Code)
	}
}

func TestDeleteVpc_Success(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-del/state", "running")
	req := httptest.NewRequest(http.MethodDelete, "/vpcs/vpc-del", nil)
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d: %s", w.Code, w.Body.String())
	}
	var result VPC
	json.NewDecoder(w.Body).Decode(&result)
	if result.State != "deleting" {
		t.Errorf("state attendu deleting, obtenu %q", result.State)
	}
}

func TestDeleteVpc_NotFound(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/vpcs/inexistant", nil)
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("attendu 404, obtenu %d", w.Code)
	}
}

func TestDeleteVpc_ConflictWhileCreating(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-wip/state", "creating")
	req := httptest.NewRequest(http.MethodDelete, "/vpcs/vpc-wip", nil)
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("attendu 409, obtenu %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteVpc_AllowedFromError(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-ko/state", "error")
	req := httptest.NewRequest(http.MethodDelete, "/vpcs/vpc-ko", nil)
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d: %s", w.Code, w.Body.String())
	}
	var result VPC
	json.NewDecoder(w.Body).Decode(&result)
	if result.State != "deleting" {
		t.Errorf("state attendu deleting, obtenu %q", result.State)
	}
}

func TestDeleteVpc_BlockedByActiveSubnet(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-busy/state", "running")
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-busy")
	req := httptest.NewRequest(http.MethodDelete, "/vpcs/vpc-busy", nil)
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("attendu 409, obtenu %d: %s", w.Code, w.Body.String())
	}
}

func TestVpcByName_InvalidMethod(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	req := httptest.NewRequest(http.MethodPut, "/vpcs/vpc-1", nil)
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("attendu 405, obtenu %d", w.Code)
	}
}

func newBGPTestServer(t *testing.T) (*Server, *badger.DB) {
	cfg := &configuration.Config{DefaultInterface: "br-test"}
	cfg.BGP.ASN = 64600
	cfg.BGP.RouteReflectors = []string{"10.255.255.1"}
	return newTestServerWith(t, cfg)
}

func postVpcJSON(s *Server, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.VpcsHandler(w, httptest.NewRequest(http.MethodPost, "/vpcs", strings.NewReader(body)))
	return w
}

func TestPostVpc_WithL3VNI(t *testing.T) {
	s, db := newBGPTestServer(t)
	w := postVpcJSON(s, `{"name":"vp-1","cidr":"10.0.0.0/16","l3vni":4001}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"l3vni":4001`) {
		t.Errorf("réponse sans l3vni : %s", w.Body.String())
	}
	if got, _ := kv.GetFromDB(db, "vpc/vp-1/l3vni"); got != "4001" {
		t.Errorf("l3vni en DB = %q", got)
	}
}

func TestPostVpc_WithoutL3VNIOmitsTheField(t *testing.T) {
	s, _ := newBGPTestServer(t)
	w := postVpcJSON(s, `{"name":"vp-1","cidr":"10.0.0.0/16"}`)
	if w.Code != http.StatusAccepted || strings.Contains(w.Body.String(), "l3vni") {
		t.Errorf("%d : %s", w.Code, w.Body.String())
	}
}

func TestPostVpc_RejectsAnInvalidL3VNIAsABadRequest(t *testing.T) {
	s, db := newBGPTestServer(t)
	for _, body := range []string{
		`{"name":"vp-1","cidr":"10.0.0.0/16","l3vni":0}`,
		`{"name":"vp-1","cidr":"10.0.0.0/16","l3vni":255}`,
		`{"name":"vp-1","cidr":"10.0.0.0/16","l3vni":65536}`,
		`{"name":"vp-1","cidr":"10.0.0.0/16","l3vni":16777216}`,
		`{"name":"vp-1","cidr":"10.0.0.0/16","l3vni":-1}`,
	} {
		if w := postVpcJSON(s, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s : attendu 400, obtenu %d: %s", body, w.Code, w.Body.String())
		}
	}
	if entries, _ := kv.ListByPrefix(db, "vpc/"); len(entries) != 0 {
		t.Errorf("écritures malgré le refus : %v", entries)
	}
}

func TestPostVpc_L3VNIWithoutBGPIsAConflict(t *testing.T) {
	s, _ := newTestServer(t)
	w := postVpcJSON(s, `{"name":"vp-1","cidr":"10.0.0.0/16","l3vni":4001}`)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "requires the bgp section") {
		t.Errorf("%d : %s", w.Code, w.Body.String())
	}
}

func TestGetAndListVpc_ReturnTheL3VNI(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vp-1/state", "running")
	kv.AddInDB(db, "vpc/vp-1/cidr", "10.0.0.0/16")
	kv.AddInDB(db, "vpc/vp-1/l3vni", "4001")
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, httptest.NewRequest(http.MethodGet, "/vpcs/vp-1", nil))
	var got VPC
	json.NewDecoder(w.Body).Decode(&got)
	if got.L3VNI != 4001 {
		t.Errorf("GET : l3vni = %d", got.L3VNI)
	}
	w = httptest.NewRecorder()
	s.VpcsHandler(w, httptest.NewRequest(http.MethodGet, "/vpcs", nil))
	var list []VPC
	json.NewDecoder(w.Body).Decode(&list)
	if len(list) != 1 || list[0].L3VNI != 4001 {
		t.Errorf("liste : %+v", list)
	}
}

package agentapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
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
	kv.AddInDB(db, "vpc/v1/state", "created")
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
	body, _ := json.Marshal(VPCCreateRequest{Name: "vpc-new"})
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
}

func TestPostVpc_MissingName(t *testing.T) {
	s, _ := newTestServer(t)
	body, _ := json.Marshal(VPCCreateRequest{})
	w := httptest.NewRecorder()
	s.VpcsHandler(w, httptest.NewRequest(http.MethodPost, "/vpcs", bytes.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("attendu 400, obtenu %d", w.Code)
	}
}

func TestPostVpc_Duplicate(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-exist/state", "created")
	body, _ := json.Marshal(VPCCreateRequest{Name: "vpc-exist"})
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
	kv.AddInDB(db, "vpc/vpc-1/state", "created")
	req := httptest.NewRequest(http.MethodGet, "/vpcs/vpc-1", nil)
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("attendu 200, obtenu %d", w.Code)
	}
	var result VPC
	json.NewDecoder(w.Body).Decode(&result)
	if result.Name != "vpc-1" || result.State != "created" {
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
	kv.AddInDB(db, "vpc/vpc-del/state", "created")
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

func TestDeleteVpc_BlockedByActiveSubnet(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "vpc/vpc-busy/state", "created")
	kv.AddInDB(db, "subnet/sn-1/state", "created")
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
	kv.AddInDB(db, "vpc/vpc-1/state", "created")
	req := httptest.NewRequest(http.MethodPut, "/vpcs/vpc-1", nil)
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("attendu 405, obtenu %d", w.Code)
	}
}

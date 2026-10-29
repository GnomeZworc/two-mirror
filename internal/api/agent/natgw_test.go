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

func seedRoutedVPC(t *testing.T, db *badger.DB) {
	t.Helper()
	kv.AddInDB(db, "vpc/vp-s7/state", "running")
	kv.AddInDB(db, "vpc/vp-s7/cidr", "10.241.0.0/16")
	kv.AddInDB(db, "vpc/vp-s7/l3vni", "2700")
}

func postNATGateway(t *testing.T, s *Server, req NATGatewayCreateRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.NATGatewaysHandler(w, httptest.NewRequest(http.MethodPost, "/nat-gateways", bytes.NewReader(body)))
	return w
}

func validNATGatewayRequest() NATGatewayCreateRequest {
	return NATGatewayCreateRequest{Name: "ng-1", VPC: "vp-s7", IP: "198.51.100.50/24", Gateway: "198.51.100.1"}
}

func TestPostNATGateway_Accepted(t *testing.T) {
	s, db := newTestServer(t)
	seedRoutedVPC(t, db)
	w := postNATGateway(t, s, validNATGatewayRequest())
	if w.Code != http.StatusAccepted {
		t.Fatalf("attendu 202, obtenu %d : %s", w.Code, w.Body.String())
	}
	var got NATGateway
	json.NewDecoder(w.Body).Decode(&got)
	want := NATGateway{Name: "ng-1", State: "creating", VPC: "vp-s7", IP: "198.51.100.50/24", Gateway: "198.51.100.1"}
	if got != want {
		t.Errorf("réponse %+v, attendu %+v", got, want)
	}
}

func TestPostNATGateway_BadRequest(t *testing.T) {
	cases := []struct {
		label  string
		mutate func(*NATGatewayCreateRequest)
		msg    string
	}{
		{"nom absent", func(r *NATGatewayCreateRequest) { r.Name = "" }, "name is required"},
		{"vpc absent", func(r *NATGatewayCreateRequest) { r.VPC = "" }, "vpc is required"},
		{"ip absente", func(r *NATGatewayCreateRequest) { r.IP = "" }, "ip is required"},
		{"gateway absente", func(r *NATGatewayCreateRequest) { r.Gateway = "" }, "gateway is required"},
		{"ip sans préfixe", func(r *NATGatewayCreateRequest) { r.IP = "198.51.100.50" }, "invalid ip"},
		{"ip IPv6", func(r *NATGatewayCreateRequest) { r.IP = "2001:db8::50/64" }, "invalid ip"},
		{"gateway invalide", func(r *NATGatewayCreateRequest) { r.Gateway = "bordure" }, "invalid gateway"},
		{"gateway IPv6", func(r *NATGatewayCreateRequest) { r.Gateway = "2001:db8::1" }, "invalid gateway"},
	}
	for _, c := range cases {
		s, db := newTestServer(t)
		seedRoutedVPC(t, db)
		req := validNATGatewayRequest()
		c.mutate(&req)
		w := postNATGateway(t, s, req)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), c.msg) {
			t.Errorf("%s : %d %s, 400 « %s » attendu", c.label, w.Code, w.Body.String(), c.msg)
		}
		if _, err := kv.GetFromDB(db, "nat-gateway/ng-1/state"); err == nil {
			t.Errorf("%s : passerelle écrite malgré le refus", c.label)
		}
	}
}

func TestPostNATGateway_InvalidBody(t *testing.T) {
	s, _ := newTestServer(t)
	w := httptest.NewRecorder()
	s.NATGatewaysHandler(w, httptest.NewRequest(http.MethodPost, "/nat-gateways", bytes.NewReader([]byte("not json"))))
	if w.Code != http.StatusBadRequest {
		t.Errorf("attendu 400, obtenu %d", w.Code)
	}
}

func TestPostNATGateway_ConflictWithoutRunningRoutedVPC(t *testing.T) {
	s, _ := newTestServer(t)
	w := postNATGateway(t, s, validNATGatewayRequest())
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `vpc \"vp-s7\" not found`) {
		t.Errorf("%d %s, 409 attendu", w.Code, w.Body.String())
	}
}

func TestListNATGateways(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "nat-gateway/ng-1/state", "running")
	kv.AddInDB(db, "nat-gateway/ng-1/vpc", "vp-s7")
	kv.AddInDB(db, "nat-gateway/ng-1/ip", "198.51.100.50/24")
	kv.AddInDB(db, "nat-gateway/ng-1/gateway", "198.51.100.1")
	w := httptest.NewRecorder()
	s.NATGatewaysHandler(w, httptest.NewRequest(http.MethodGet, "/nat-gateways", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("attendu 200, obtenu %d", w.Code)
	}
	var got []NATGateway
	json.NewDecoder(w.Body).Decode(&got)
	want := NATGateway{Name: "ng-1", State: "running", VPC: "vp-s7", IP: "198.51.100.50/24", Gateway: "198.51.100.1"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("liste %+v, attendu [%+v]", got, want)
	}
}

func TestListNATGateways_Empty(t *testing.T) {
	s, _ := newTestServer(t)
	w := httptest.NewRecorder()
	s.NATGatewaysHandler(w, httptest.NewRequest(http.MethodGet, "/nat-gateways", nil))
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("%d %q, 200 [] attendu", w.Code, w.Body.String())
	}
}

func TestGetNATGateway(t *testing.T) {
	s, db := newTestServer(t)
	kv.AddInDB(db, "nat-gateway/ng-1/state", "running")
	kv.AddInDB(db, "nat-gateway/ng-1/vpc", "vp-s7")
	w := httptest.NewRecorder()
	s.NATGatewayByNameHandler(w, httptest.NewRequest(http.MethodGet, "/nat-gateways/ng-1", nil))
	var got NATGateway
	json.NewDecoder(w.Body).Decode(&got)
	if w.Code != http.StatusOK || got.Name != "ng-1" || got.State != "running" || got.VPC != "vp-s7" {
		t.Errorf("%d %+v", w.Code, got)
	}
	w = httptest.NewRecorder()
	s.NATGatewayByNameHandler(w, httptest.NewRequest(http.MethodGet, "/nat-gateways/ng-x", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("passerelle absente : %d, 404 attendu", w.Code)
	}
}

func TestDeleteNATGateway(t *testing.T) {
	cases := map[string]int{"running": http.StatusAccepted, "error": http.StatusAccepted, "creating": http.StatusConflict, "": http.StatusNotFound}
	for st, want := range cases {
		s, db := newTestServer(t)
		if st != "" {
			kv.AddInDB(db, "nat-gateway/ng-1/state", st)
			kv.AddInDB(db, "nat-gateway/ng-1/vpc", "vp-s7")
		}
		w := httptest.NewRecorder()
		s.NATGatewayByNameHandler(w, httptest.NewRequest(http.MethodDelete, "/nat-gateways/ng-1", nil))
		if w.Code != want {
			t.Errorf("état %q : %d, %d attendu (%s)", st, w.Code, want, w.Body.String())
		}
	}
}

func TestDeleteVpc_BlockedByNATGateway(t *testing.T) {
	s, db := newTestServer(t)
	seedRoutedVPC(t, db)
	kv.AddInDB(db, "nat-gateway/ng-1/state", "running")
	kv.AddInDB(db, "nat-gateway/ng-1/vpc", "vp-s7")
	w := httptest.NewRecorder()
	s.VpcByNameHandler(w, httptest.NewRequest(http.MethodDelete, "/vpcs/vp-s7", nil))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `nat gateway \"ng-1\" must be deleted`) {
		t.Errorf("%d %s, 409 attendu", w.Code, w.Body.String())
	}
}

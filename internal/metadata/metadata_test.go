package metadata

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func newCfg() NoCloudConfig {
	return NoCloudConfig{
		VpcName:  "vpc-test",
		BindIP:   "169.254.169.254",
		BindPort: "80",
		Name:     "vm1",
		Password: "s3cr3t",
		SSHKEY:   "ssh-ed25519 AAAA... user@host",
	}
}

func newTestDB(t *testing.T) interface{ Close() error } {
	t.Helper()
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	return db
}

// --- RenderConfig ---

func TestRenderConfig_MetaData(t *testing.T) {
	cfg := newCfg()
	out, err := RenderConfig("templates/meta-data.tmpl", cfg)
	if err != nil {
		t.Fatalf("RenderConfig meta-data : %v", err)
	}
	if !strings.Contains(out, "instance-id: vm1") {
		t.Errorf("instance-id absent :\n%s", out)
	}
	if !strings.Contains(out, "local-hostname: vm1") {
		t.Errorf("local-hostname absent :\n%s", out)
	}
}

func TestRenderConfig_VendorData_ContainsPassword(t *testing.T) {
	cfg := newCfg()
	out, err := RenderConfig("templates/vendor-data.tmpl", cfg)
	if err != nil {
		t.Fatalf("RenderConfig vendor-data : %v", err)
	}
	if !strings.Contains(out, "s3cr3t") {
		t.Errorf("password absent du vendor-data :\n%s", out)
	}
}

func TestRenderConfig_VendorData_ContainsSSHKey(t *testing.T) {
	cfg := newCfg()
	out, err := RenderConfig("templates/vendor-data.tmpl", cfg)
	if err != nil {
		t.Fatalf("RenderConfig vendor-data : %v", err)
	}
	if !strings.Contains(out, "ssh-ed25519 AAAA... user@host") {
		t.Errorf("clé SSH absente du vendor-data :\n%s", out)
	}
}

func TestRenderConfig_NetworkConfig(t *testing.T) {
	cfg := newCfg()
	out, err := RenderConfig("templates/network-config.tmpl", cfg)
	if err != nil {
		t.Fatalf("RenderConfig network-config : %v", err)
	}
	if !strings.Contains(out, "dhcp4: true") {
		t.Errorf("dhcp4 absent du network-config :\n%s", out)
	}
}

func TestRenderConfig_UserData(t *testing.T) {
	cfg := newCfg()
	out, err := RenderConfig("templates/user-data.tmpl", cfg)
	if err != nil {
		t.Fatalf("RenderConfig user-data : %v", err)
	}
	if !strings.Contains(out, "passwd -d root") {
		t.Errorf("user-data inattendu :\n%s", out)
	}
}

func TestRenderConfig_InvalidTemplate(t *testing.T) {
	_, err := RenderConfig("templates/inexistant.tmpl", newCfg())
	if err == nil {
		t.Error("RenderConfig devrait retourner une erreur pour un template inexistant")
	}
}

func TestRenderConfig_SpecialCharsInName(t *testing.T) {
	cfg := newCfg()
	cfg.Name = "vm-prod-01"
	out, err := RenderConfig("templates/meta-data.tmpl", cfg)
	if err != nil {
		t.Fatalf("RenderConfig : %v", err)
	}
	if !strings.Contains(out, "vm-prod-01") {
		t.Errorf("nom vm-prod-01 absent :\n%s", out)
	}
}

// --- LoadNcCloudInDB / UnLoadNoCloudInDB ---

func TestLoadNcCloudInDB_StoresAllKeys(t *testing.T) {
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })

	cfg := newCfg()
	LoadNcCloudInDB(cfg, db)

	keys := []string{
		"metadata/vm1/meta-data",
		"metadata/vm1/user-data",
		"metadata/vm1/network-config",
		"metadata/vm1/vendor-data",
		"metadata/vm1/vpc",
		"metadata/vm1/bind_ip",
		"metadata/vm1/bind_port",
	}
	for _, key := range keys {
		val, err := kv.GetFromDB(db, key)
		if err != nil {
			t.Errorf("clé %q absente après LoadNcCloudInDB : %v", key, err)
		}
		if val == "" && key != "metadata/vm1/user-data" {
			t.Errorf("clé %q vide après LoadNcCloudInDB", key)
		}
	}
}

func TestLoadNcCloudInDB_VpcAndBindValues(t *testing.T) {
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })

	cfg := newCfg()
	LoadNcCloudInDB(cfg, db)

	vpc, _ := kv.GetFromDB(db, "metadata/vm1/vpc")
	if vpc != "vpc-test" {
		t.Errorf("vpc attendu %q, obtenu %q", "vpc-test", vpc)
	}

	ip, _ := kv.GetFromDB(db, "metadata/vm1/bind_ip")
	if ip != "169.254.169.254" {
		t.Errorf("bind_ip attendu %q, obtenu %q", "169.254.169.254", ip)
	}

	port, _ := kv.GetFromDB(db, "metadata/vm1/bind_port")
	if port != "80" {
		t.Errorf("bind_port attendu %q, obtenu %q", "80", port)
	}
}

func TestUnLoadNoCloudInDB_RemovesAllKeys(t *testing.T) {
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })

	cfg := newCfg()
	LoadNcCloudInDB(cfg, db)
	UnLoadNoCloudInDB("vm1", db)

	keys := []string{
		"metadata/vm1/meta-data",
		"metadata/vm1/user-data",
		"metadata/vm1/network-config",
		"metadata/vm1/vendor-data",
		"metadata/vm1/vpc",
		"metadata/vm1/bind_ip",
		"metadata/vm1/bind_port",
	}
	for _, key := range keys {
		_, err := kv.GetFromDB(db, key)
		if err == nil {
			t.Errorf("clé %q devrait être supprimée après UnLoadNoCloudInDB", key)
		}
	}
}

// --- getIP ---

func TestGetIP_ValidHostPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:4567"
	if ip := getIP(req); ip != "10.0.0.1" {
		t.Errorf("attendu 10.0.0.1, obtenu %q", ip)
	}
}

func TestGetIP_IPv6(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[::1]:8080"
	if ip := getIP(req); ip != "::1" {
		t.Errorf("attendu ::1, obtenu %q", ip)
	}
}

func TestGetIP_NoPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1"
	if ip := getIP(req); ip != "10.0.0.1" {
		t.Errorf("attendu RemoteAddr brut, obtenu %q", ip)
	}
}

// --- rootHandler ---

func TestRootHandler_UserData(t *testing.T) {
	data = NoCloudData{UserData: "userdata-content"}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/user-data", nil)
	rootHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("attendu 200, obtenu %d", w.Code)
	}
	if body := w.Body.String(); body != "userdata-content" {
		t.Errorf("body inattendu : %q", body)
	}
}

func TestRootHandler_MetaData(t *testing.T) {
	data = NoCloudData{MetaData: "metadata-content"}
	w := httptest.NewRecorder()
	rootHandler(w, httptest.NewRequest(http.MethodGet, "/meta-data", nil))
	if w.Code != http.StatusOK {
		t.Errorf("attendu 200, obtenu %d", w.Code)
	}
	if body := w.Body.String(); body != "metadata-content" {
		t.Errorf("body inattendu : %q", body)
	}
}

func TestRootHandler_NetworkConfig(t *testing.T) {
	data = NoCloudData{NetworkConfig: "network-content"}
	w := httptest.NewRecorder()
	rootHandler(w, httptest.NewRequest(http.MethodGet, "/network-config", nil))
	if w.Code != http.StatusOK {
		t.Errorf("attendu 200, obtenu %d", w.Code)
	}
	if body := w.Body.String(); body != "network-content" {
		t.Errorf("body inattendu : %q", body)
	}
}

func TestRootHandler_VendorData(t *testing.T) {
	data = NoCloudData{VendorData: "vendor-content"}
	w := httptest.NewRecorder()
	rootHandler(w, httptest.NewRequest(http.MethodGet, "/vendor-data", nil))
	if w.Code != http.StatusOK {
		t.Errorf("attendu 200, obtenu %d", w.Code)
	}
	if body := w.Body.String(); body != "vendor-content" {
		t.Errorf("body inattendu : %q", body)
	}
}

func TestRootHandler_UnknownPath(t *testing.T) {
	data = NoCloudData{}
	w := httptest.NewRecorder()
	rootHandler(w, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("attendu 404, obtenu %d", w.Code)
	}
}

func TestRootHandler_ContentType(t *testing.T) {
	data = NoCloudData{MetaData: "x"}
	w := httptest.NewRecorder()
	rootHandler(w, httptest.NewRequest(http.MethodGet, "/meta-data", nil))
	if ct := w.Header().Get("Content-Type"); ct != "text/yaml" {
		t.Errorf("Content-Type attendu text/yaml, obtenu %q", ct)
	}
}

// --- UnLoadNoCloudInDB_DoesNotAffectOtherVMs ---

func TestUnLoadNoCloudInDB_DoesNotAffectOtherVMs(t *testing.T) {
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })

	cfg1 := newCfg()
	cfg2 := newCfg()
	cfg2.Name = "vm2"
	LoadNcCloudInDB(cfg1, db)
	LoadNcCloudInDB(cfg2, db)

	UnLoadNoCloudInDB("vm1", db)

	_, err := kv.GetFromDB(db, "metadata/vm2/vpc")
	if err != nil {
		t.Errorf("vm2 ne devrait pas être supprimée : %v", err)
	}
}

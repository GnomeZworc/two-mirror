package metadata

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func useTestDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
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

func readTestFile(t *testing.T, dir, vmName, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, vmName, name))
	if err != nil {
		t.Errorf("fichier %q absent après LoadNcCloudInDB : %v", name, err)
		return ""
	}
	return string(b)
}

func TestLoadNcCloudInDB_StoresAllFiles(t *testing.T) {
	dir := useTestDir(t)
	LoadNcCloudInDB(newCfg(), dir)

	files := []string{"meta-data", "user-data", "network-config", "vendor-data", "vpc", "bind_ip", "bind_port"}
	for _, f := range files {
		path := filepath.Join(dir, "vm1", f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("fichier %q absent : %v", f, err)
		}
	}
}

func TestLoadNcCloudInDB_VpcAndBindValues(t *testing.T) {
	dir := useTestDir(t)
	LoadNcCloudInDB(newCfg(), dir)

	if vpc := readTestFile(t, dir, "vm1", "vpc"); vpc != "vpc-test" {
		t.Errorf("vpc attendu %q, obtenu %q", "vpc-test", vpc)
	}
	if ip := readTestFile(t, dir, "vm1", "bind_ip"); ip != "169.254.169.254" {
		t.Errorf("bind_ip attendu %q, obtenu %q", "169.254.169.254", ip)
	}
	if port := readTestFile(t, dir, "vm1", "bind_port"); port != "80" {
		t.Errorf("bind_port attendu %q, obtenu %q", "80", port)
	}
}

func TestUnLoadNoCloudInDB_RemovesAllFiles(t *testing.T) {
	dir := useTestDir(t)
	LoadNcCloudInDB(newCfg(), dir)
	UnLoadNoCloudInDB("vm1", dir)

	if _, err := os.Stat(filepath.Join(dir, "vm1")); !os.IsNotExist(err) {
		t.Error("répertoire vm1 devrait être supprimé après UnLoadNoCloudInDB")
	}
}

func TestUnLoadNoCloudInDB_DoesNotAffectOtherVMs(t *testing.T) {
	dir := useTestDir(t)

	cfg1 := newCfg()
	cfg2 := newCfg()
	cfg2.Name = "vm2"
	LoadNcCloudInDB(cfg1, dir)
	LoadNcCloudInDB(cfg2, dir)

	UnLoadNoCloudInDB("vm1", dir)

	if _, err := os.Stat(filepath.Join(dir, "vm2", "vpc")); err != nil {
		t.Errorf("vm2 ne devrait pas être supprimée : %v", err)
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

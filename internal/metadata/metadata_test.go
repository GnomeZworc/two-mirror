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


func mustWrite(t *testing.T, cfg NoCloudConfig, dir string) {
	t.Helper()
	if err := WriteNoCloudFiles(cfg, dir); err != nil {
		t.Fatalf("WriteNoCloudFiles : %v", err)
	}
}

func mustRemove(t *testing.T, vmName, dir string) {
	t.Helper()
	if err := RemoveNoCloudFiles(vmName, dir); err != nil {
		t.Fatalf("RemoveNoCloudFiles : %v", err)
	}
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

func TestRenderConfig_UserData_DefaultIsEmpty(t *testing.T) {
	doc, err := renderDocument(DocUserData, newCfg())
	if err != nil {
		t.Fatalf("renderDocument user-data : %v", err)
	}
	if doc != "" {
		t.Errorf("le user-data par défaut doit être vide, obtenu :\n%q", doc)
	}
}

func TestRenderConfig_UserData_NeverTouchesRootPassword(t *testing.T) {
	doc, err := renderDocument(DocUserData, newCfg())
	if err != nil {
		t.Fatalf("renderDocument user-data : %v", err)
	}
	if strings.Contains(doc, "passwd -d root") {
		t.Errorf("l'agent ne doit imposer aucune modification du compte root :\n%s", doc)
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

// --- WriteNoCloudFiles / RemoveNoCloudFiles ---

func readTestFile(t *testing.T, dir, vmName, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, vmName, name))
	if err != nil {
		t.Errorf("fichier %q absent après WriteNoCloudFiles : %v", name, err)
		return ""
	}
	return string(b)
}

func TestWriteNoCloudFiles_StoresAllFiles(t *testing.T) {
	dir := useTestDir(t)
	mustWrite(t, newCfg(), dir)

	files := []string{"meta-data", "user-data", "network-config", "vendor-data", "vpc", "bind_ip", "bind_port"}
	for _, f := range files {
		path := filepath.Join(dir, "vm1", f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("fichier %q absent : %v", f, err)
		}
	}
}

func TestWriteNoCloudFiles_VpcAndBindValues(t *testing.T) {
	dir := useTestDir(t)
	mustWrite(t, newCfg(), dir)

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

func TestRemoveNoCloudFiles_RemovesAllFiles(t *testing.T) {
	dir := useTestDir(t)
	mustWrite(t, newCfg(), dir)
	mustRemove(t, "vm1", dir)

	if _, err := os.Stat(filepath.Join(dir, "vm1")); !os.IsNotExist(err) {
		t.Error("répertoire vm1 devrait être supprimé après RemoveNoCloudFiles")
	}
}

func TestRemoveNoCloudFiles_DoesNotAffectOtherVMs(t *testing.T) {
	dir := useTestDir(t)

	cfg1 := newCfg()
	cfg2 := newCfg()
	cfg2.Name = "vm2"
	mustWrite(t, cfg1, dir)
	mustWrite(t, cfg2, dir)

	mustRemove(t, "vm1", dir)

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

// --- vendor-data : blocs conditionnels ---

func vendorData(t *testing.T, password, sshkey string) string {
	t.Helper()
	cfg := newCfg()
	cfg.Password = password
	cfg.SSHKEY = sshkey
	out, err := renderDocument(DocVendorData, cfg)
	if err != nil {
		t.Fatalf("renderDocument vendor-data : %v", err)
	}
	return out
}

func TestVendorData_NoCredentials_EmitsNothing(t *testing.T) {
	out := vendorData(t, "", "")
	if out != "" {
		t.Errorf("sans mot de passe ni clé, aucun compte ne doit être créé :\n%s", out)
	}
}

func TestVendorData_NoCredentials_NeverEmitsEmptyValues(t *testing.T) {
	out := vendorData(t, "", "")
	for _, forbidden := range []string{`passwd: ""`, `- ""`, "lock_passwd: false"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("valeur vide %q émise :\n%s", forbidden, out)
		}
	}
}

func TestVendorData_PasswordOnly(t *testing.T) {
	out := vendorData(t, "$6$rounds$hash", "")
	if !strings.Contains(out, `passwd: "$6$rounds$hash"`) {
		t.Errorf("hash absent :\n%s", out)
	}
	if !strings.Contains(out, "lock_passwd: false") {
		t.Errorf("un compte avec mot de passe doit être déverrouillé :\n%s", out)
	}
	if strings.Contains(out, "ssh_authorized_keys") {
		t.Errorf("aucune clé fournie, le bloc ne doit pas apparaître :\n%s", out)
	}
}

func TestVendorData_SSHKeyOnly_LocksPassword(t *testing.T) {
	out := vendorData(t, "", "ssh-ed25519 AAAA user@host")
	if !strings.Contains(out, "ssh-ed25519 AAAA user@host") {
		t.Errorf("clé absente :\n%s", out)
	}
	if strings.Contains(out, "\n    passwd:") {
		t.Errorf("aucun mot de passe fourni, le champ ne doit pas apparaître :\n%s", out)
	}
	if !strings.Contains(out, "lock_passwd: true") {
		t.Errorf("sans mot de passe le compte doit rester verrouillé, sinon il devient un compte sudo sans mot de passe :\n%s", out)
	}
}

func TestVendorData_BothCredentials(t *testing.T) {
	out := vendorData(t, "$6$hash", "ssh-ed25519 AAAA user@host")
	for _, expected := range []string{"#cloud-config", "name: syonad", `passwd: "$6$hash"`, "lock_passwd: false", "ssh-ed25519 AAAA user@host"} {
		if !strings.Contains(out, expected) {
			t.Errorf("%q absent :\n%s", expected, out)
		}
	}
}

func TestVendorData_CloudConfigHeaderIsFirstLine(t *testing.T) {
	out := vendorData(t, "$6$hash", "")
	if !strings.HasPrefix(out, "#cloud-config\n") {
		t.Errorf("cloud-init exige #cloud-config en première ligne :\n%q", out)
	}
}

// --- documents fournis par l'appelant ---

func TestRenderDocument_VerbatimOverridesTemplate(t *testing.T) {
	cfg := newCfg()
	supplied := "#cloud-config\npackages:\n  - nginx\n"
	cfg.Documents = map[string]string{DocUserData: supplied}

	out, err := renderDocument(DocUserData, cfg)
	if err != nil {
		t.Fatalf("renderDocument : %v", err)
	}
	if out != supplied {
		t.Errorf("le document fourni doit être écrit verbatim :\nattendu %q\nobtenu  %q", supplied, out)
	}
}

func TestRenderDocument_EmptySuppliedDocumentIsHonoured(t *testing.T) {
	cfg := newCfg()
	cfg.Documents = map[string]string{DocVendorData: ""}

	out, err := renderDocument(DocVendorData, cfg)
	if err != nil {
		t.Fatalf("renderDocument : %v", err)
	}
	if out != "" {
		t.Errorf("un document explicitement vide ne doit pas retomber sur le template :\n%s", out)
	}
}

func TestWriteNoCloudFiles_WritesSuppliedDocument(t *testing.T) {
	dir := useTestDir(t)
	cfg := newCfg()
	cfg.Documents = map[string]string{DocUserData: "#cloud-config\nruncmd:\n  - [ls]\n"}
	mustWrite(t, cfg, dir)

	if got := readTestFile(t, dir, "vm1", "user-data"); got != cfg.Documents[DocUserData] {
		t.Errorf("user-data servi différent de celui fourni :\n%q", got)
	}
}

// --- remontée des erreurs ---

func TestWriteNoCloudFiles_ReturnsErrorOnUnwritableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignore les permissions de fichiers")
	}
	dir := useTestDir(t)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("chmod : %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })

	if err := WriteNoCloudFiles(newCfg(), dir); err == nil {
		t.Error("une écriture impossible doit remonter une erreur : une VM ne doit jamais démarrer sans métadonnées en silence")
	}
}

func TestRemoveNoCloudFiles_AbsentDirIsNotAnError(t *testing.T) {
	if err := RemoveNoCloudFiles("jamais-creee", useTestDir(t)); err != nil {
		t.Errorf("supprimer une VM sans fichiers ne doit pas échouer : %v", err)
	}
}

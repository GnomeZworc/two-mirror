package dhcp

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const subName = "vp-admin_br-000001"

func TestWriteReservations_WritesOneLinePerInterface(t *testing.T) {
	dir := t.TempDir()
	res := []Reservation{
		{MAC: "00:22:33:00:01:02", IP: "10.1.1.2"},
		{MAC: "00:22:33:00:02:07", IP: "10.1.2.7"},
	}
	if err := WriteReservations(dir, subName, "i-web", res); err != nil {
		t.Fatalf("WriteReservations : %v", err)
	}

	content, err := os.ReadFile(filepath.Join(HostsDir(dir, subName), "i-web"))
	if err != nil {
		t.Fatalf("fichier absent : %v", err)
	}
	want := "00:22:33:00:01:02,10.1.1.2\n00:22:33:00:02:07,10.1.2.7\n"
	if string(content) != want {
		t.Errorf("contenu attendu %q, obtenu %q", want, content)
	}
}

func TestWriteReservations_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	if err := WriteReservations(dir, subName, "i-web", []Reservation{{MAC: "aa", IP: "10.0.0.1"}}); err != nil {
		t.Fatalf("WriteReservations : %v", err)
	}
	if fi, err := os.Stat(HostsDir(dir, subName)); err != nil || !fi.IsDir() {
		t.Errorf("hosts.d non créé : %v", err)
	}
}

func TestWriteReservations_EmptyIsAnError(t *testing.T) {
	if err := WriteReservations(t.TempDir(), subName, "i-web", nil); err == nil {
		t.Error("sans réservation la VM n'obtiendrait aucune adresse : il faut échouer, pas écrire un fichier vide")
	}
}

func TestWriteReservations_IncompleteIsAnError(t *testing.T) {
	cases := []Reservation{{MAC: "", IP: "10.0.0.1"}, {MAC: "aa:bb", IP: ""}}
	for _, r := range cases {
		if err := WriteReservations(t.TempDir(), subName, "i-web", []Reservation{r}); err == nil {
			t.Errorf("réservation incomplète acceptée : %+v", r)
		}
	}
}

func TestWriteReservations_Overwrites(t *testing.T) {
	dir := t.TempDir()
	_ = WriteReservations(dir, subName, "i-web", []Reservation{{MAC: "aa", IP: "10.0.0.1"}})
	if err := WriteReservations(dir, subName, "i-web", []Reservation{{MAC: "bb", IP: "10.0.0.2"}}); err != nil {
		t.Fatalf("WriteReservations : %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(HostsDir(dir, subName), "i-web"))
	if string(content) != "bb,10.0.0.2\n" {
		t.Errorf("la réécriture doit remplacer, obtenu %q", content)
	}
}

func TestRemoveReservations_RemovesBothFiles(t *testing.T) {
	dir := t.TempDir()
	_ = WriteReservations(dir, subName, "i-web", []Reservation{{MAC: "aa", IP: "10.0.0.1"}})
	if err := os.MkdirAll(OptsDir(dir, subName), 0755); err != nil {
		t.Fatal(err)
	}
	optsFile := filepath.Join(OptsDir(dir, subName), "i-web")
	if err := os.WriteFile(optsFile, []byte("tag:i-web,3,10.0.0.1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveReservations(dir, subName, "i-web"); err != nil {
		t.Fatalf("RemoveReservations : %v", err)
	}
	for _, p := range []string{filepath.Join(HostsDir(dir, subName), "i-web"), optsFile} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s aurait dû être supprimé", p)
		}
	}
}

func TestRemoveReservations_AbsentIsNotAnError(t *testing.T) {
	if err := RemoveReservations(t.TempDir(), subName, "jamais-creee"); err != nil {
		t.Errorf("supprimer une VM sans réservation ne doit pas échouer : %v", err)
	}
}

func TestRemoveReservations_LeavesOtherVMs(t *testing.T) {
	dir := t.TempDir()
	_ = WriteReservations(dir, subName, "i-web", []Reservation{{MAC: "aa", IP: "10.0.0.1"}})
	_ = WriteReservations(dir, subName, "i-db", []Reservation{{MAC: "bb", IP: "10.0.0.2"}})

	if err := RemoveReservations(dir, subName, "i-web"); err != nil {
		t.Fatalf("RemoveReservations : %v", err)
	}
	if _, err := os.Stat(filepath.Join(HostsDir(dir, subName), "i-db")); err != nil {
		t.Errorf("i-db ne devait pas être touchée : %v", err)
	}
}

func TestRemoveSubnetDirs(t *testing.T) {
	dir := t.TempDir()
	_ = WriteReservations(dir, subName, "i-web", []Reservation{{MAC: "aa", IP: "10.0.0.1"}})

	if err := RemoveSubnetDirs(dir, subName); err != nil {
		t.Fatalf("RemoveSubnetDirs : %v", err)
	}
	for _, d := range []string{HostsDir(dir, subName), OptsDir(dir, subName)} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("%s aurait dû être supprimé", d)
		}
	}
}

func TestUnitName(t *testing.T) {
	if got := UnitName(subName); got != "dnsmasq@vp-admin_br-000001.service" {
		t.Errorf("unit attendue dnsmasq@%s.service, obtenu %s", subName, got)
	}
}

// --- options par interface ---

func TestWriteReservations_WithTags(t *testing.T) {
	dir := t.TempDir()
	res := []Reservation{
		{MAC: "00:22:33:00:01:02", IP: "10.1.1.2", Tag: "i-web-0"},
		{MAC: "00:22:33:00:02:07", IP: "10.1.2.7", Tag: "i-web-1"},
	}
	if err := WriteReservations(dir, subName, "i-web", res); err != nil {
		t.Fatalf("WriteReservations : %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(HostsDir(dir, subName), "i-web"))
	want := "00:22:33:00:01:02,10.1.1.2,set:i-web-0\n00:22:33:00:02:07,10.1.2.7,set:i-web-1\n"
	if string(content) != want {
		t.Errorf("attendu %q, obtenu %q", want, content)
	}
}

func vmOptions(t *testing.T, dir string, tags []string, vpcRoute *net.IPNet) string {
	t.Helper()
	if err := WriteVMOptions(dir, subName, "i-web", tags, Config{
		InterfaceIP: net.ParseIP("10.1.1.1").To4(),
		VPCRoute:    vpcRoute,
	}); err != nil {
		t.Fatalf("WriteVMOptions : %v", err)
	}
	b, err := os.ReadFile(filepath.Join(OptsDir(dir, subName), "i-web"))
	if err != nil {
		return ""
	}
	return string(b)
}

func TestWriteVMOptions_SuppressesDefaultRoute(t *testing.T) {
	_, vpcNet, _ := net.ParseCIDR("192.168.0.0/16")
	got := vmOptions(t, t.TempDir(), []string{"i-web-1"}, vpcNet)

	if !strings.Contains(got, "tag:i-web-1,3\n") {
		t.Errorf("l'option 3 nue doit supprimer la route par défaut :\n%s", got)
	}
	if strings.Contains(got, "0.0.0.0/0") {
		t.Errorf("la route par défaut ne doit pas figurer dans l'override :\n%s", got)
	}
}

func TestWriteVMOptions_ReEmitsMetadataRoute(t *testing.T) {
	_, vpcNet, _ := net.ParseCIDR("192.168.0.0/16")
	got := vmOptions(t, t.TempDir(), []string{"i-web-1"}, vpcNet)

	if !strings.Contains(got, "169.254.169.254/32,10.1.1.1") {
		t.Errorf("un override de l'option 121 remplace la précédente en entier : sans la route metadata, la VM ne se provisionne pas\n%s", got)
	}
	if !strings.Contains(got, "192.168.0.0/16,10.1.1.1") {
		t.Errorf("la route VPC doit être réémise elle aussi :\n%s", got)
	}
}

func TestWriteVMOptions_BridgeHasNoVPCRoute(t *testing.T) {
	got := vmOptions(t, t.TempDir(), []string{"i-web-1"}, nil)

	if !strings.Contains(got, "169.254.169.254/32") {
		t.Errorf("route metadata absente :\n%s", got)
	}
	if strings.Contains(got, "192.168") {
		t.Errorf("aucune route VPC attendue en mode bridge :\n%s", got)
	}
}

func TestWriteVMOptions_OneBlockPerTag(t *testing.T) {
	got := vmOptions(t, t.TempDir(), []string{"i-web-1", "i-web-2"}, nil)

	for _, tag := range []string{"tag:i-web-1,3", "tag:i-web-2,3"} {
		if !strings.Contains(got, tag) {
			t.Errorf("%q absent :\n%s", tag, got)
		}
	}
}

func TestWriteVMOptions_NoTagRemovesFile(t *testing.T) {
	dir := t.TempDir()
	_ = vmOptions(t, dir, []string{"i-web-1"}, nil)

	if err := WriteVMOptions(dir, subName, "i-web", nil, Config{}); err != nil {
		t.Fatalf("WriteVMOptions : %v", err)
	}
	if _, err := os.Stat(filepath.Join(OptsDir(dir, subName), "i-web")); !os.IsNotExist(err) {
		t.Error("sans interface non primaire, aucun fichier d'options ne doit subsister")
	}
}

func TestWriteVMOptions_RefusesDefaultGateway(t *testing.T) {
	err := WriteVMOptions(t.TempDir(), subName, "i-web", []string{"i-web-1"}, Config{
		InterfaceIP:    net.ParseIP("10.1.1.1").To4(),
		DefaultGateway: net.ParseIP("10.1.1.254").To4(),
	})
	if err == nil {
		t.Error("ces options servent à retirer la route par défaut : en porter une est une incohérence")
	}
}

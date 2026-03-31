package dhcp

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseNet(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("ParseCIDR(%q) : %v", cidr, err)
	}
	return network
}

// --- cloneIP ---

func TestCloneIP_IsIndependent(t *testing.T) {
	ip := net.ParseIP("10.0.0.1").To4()
	clone := cloneIP(ip)
	clone[3] = 99
	if ip[3] == 99 {
		t.Error("cloneIP devrait retourner une copie indépendante")
	}
}

// --- incrementIP ---

func TestIncrementIP_Simple(t *testing.T) {
	ip := net.ParseIP("10.0.0.1").To4()
	incrementIP(ip)
	if ip.String() != "10.0.0.2" {
		t.Errorf("attendu 10.0.0.2, obtenu %s", ip)
	}
}

func TestIncrementIP_Carry(t *testing.T) {
	ip := net.ParseIP("10.0.0.255").To4()
	incrementIP(ip)
	if ip.String() != "10.0.1.0" {
		t.Errorf("attendu 10.0.1.0, obtenu %s", ip)
	}
}

// --- GenerateConfig ---

func newConf(t *testing.T, cidr string) Config {
	t.Helper()
	_, network, _ := net.ParseCIDR(cidr)
	return Config{
		Network: network,
		Gateway: net.ParseIP("192.168.1.1").To4(),
		Name:    "test",
		ConfDir: t.TempDir(),
	}
}

func TestGenerateConfig_CreatesFile(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29") // 6 hôtes
	path, err := GenerateConfig(conf)
	if err != nil {
		t.Fatalf("GenerateConfig a échoué : %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("le fichier %q n'a pas été créé", path)
	}
}

func TestGenerateConfig_FilenameMatchesName(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	path, err := GenerateConfig(conf)
	if err != nil {
		t.Fatalf("GenerateConfig a échoué : %v", err)
	}

	expected := filepath.Join(conf.ConfDir, "test.conf")
	if path != expected {
		t.Errorf("chemin attendu %q, obtenu %q", expected, path)
	}
}

func TestGenerateConfig_ContainsGateway(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	path, _ := GenerateConfig(conf)
	content, _ := os.ReadFile(path)

	if !strings.Contains(string(content), "dhcp-option=3,192.168.1.1") {
		t.Errorf("gateway absente du fichier généré :\n%s", content)
	}
}

func TestGenerateConfig_ContainsDhcpRange(t *testing.T) {
	_, network, _ := net.ParseCIDR("10.10.0.0/24")
	conf := Config{
		Network: network,
		Gateway: net.ParseIP("10.10.0.1").To4(),
		Name:    "vpc1",
		ConfDir: t.TempDir(),
	}
	path, _ := GenerateConfig(conf)
	content, _ := os.ReadFile(path)

	if !strings.Contains(string(content), "dhcp-range=10.10.0.0,static,255.255.255.0,12h") {
		t.Errorf("dhcp-range absent ou incorrect :\n%s", content)
	}
}

func TestGenerateConfig_OneHostEntryPerIP(t *testing.T) {
	// /29 = réseau + broadcast + 6 hôtes → 8 adresses
	conf := newConf(t, "10.0.0.0/29")
	path, _ := GenerateConfig(conf)
	content, _ := os.ReadFile(path)

	lines := strings.Split(string(content), "\n")
	count := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "dhcp-host=") {
			count++
		}
	}
	// /29 contient 8 adresses (0 à 7)
	if count != 8 {
		t.Errorf("attendu 8 entrées dhcp-host, obtenu %d", count)
	}
}

func TestGenerateConfig_MACPrefix(t *testing.T) {
	conf := newConf(t, "10.0.0.0/30") // 4 adresses
	path, _ := GenerateConfig(conf)
	content, _ := os.ReadFile(path)

	if !strings.Contains(string(content), "00:22:33:") {
		t.Errorf("préfixe MAC 00:22:33: absent :\n%s", content)
	}
}

func TestGenerateConfig_CreatesConfDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sous", "dossier")
	_, network, _ := net.ParseCIDR("10.0.0.0/30")
	conf := Config{
		Network: network,
		Gateway: net.ParseIP("10.0.0.1").To4(),
		Name:    "net",
		ConfDir: dir,
	}
	if _, err := GenerateConfig(conf); err != nil {
		t.Fatalf("GenerateConfig devrait créer les répertoires manquants : %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("répertoire %q non créé", dir)
	}
}

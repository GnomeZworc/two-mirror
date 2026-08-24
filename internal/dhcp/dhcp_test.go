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
	_, vpcNet, _ := net.ParseCIDR("10.0.0.0/16")
	gw := net.ParseIP("192.168.1.1").To4()
	return Config{
		Network:        network,
		InterfaceIP:    net.ParseIP("192.168.1.254").To4(),
		VPCRoute:       vpcNet,
		DefaultGateway: gw,
		Name:           "test",
		ConfDir:        t.TempDir(),
	}
}

func TestGenerateConfig_CreatesFile(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29") // 6 hôtes
	path, _, err := GenerateConfig(conf)
	if err != nil {
		t.Fatalf("GenerateConfig a échoué : %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("le fichier %q n'a pas été créé", path)
	}
}

func TestGenerateConfig_FilenameMatchesName(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	path, _, err := GenerateConfig(conf)
	if err != nil {
		t.Fatalf("GenerateConfig a échoué : %v", err)
	}

	expected := filepath.Join(conf.ConfDir, "test.conf")
	if path != expected {
		t.Errorf("chemin attendu %q, obtenu %q", expected, path)
	}
}

func TestGenerateConfig_ContainsDefaultGateway(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	path, _, _ := GenerateConfig(conf)
	content, _ := os.ReadFile(path)

	if !strings.Contains(string(content), "dhcp-option=3,192.168.1.1") {
		t.Errorf("dhcp-option=3 absente du fichier généré :\n%s", content)
	}
}

func TestGenerateConfig_NoDefaultGateway(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	conf.DefaultGateway = nil
	path, _, _ := GenerateConfig(conf)
	content, _ := os.ReadFile(path)

	if strings.Contains(string(content), "dhcp-option=3,") {
		t.Errorf("dhcp-option=3 présente alors que DefaultGateway=nil :\n%s", content)
	}
}

func TestGenerateConfig_NoDefaultGatewaySuppressesRouterOption(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	conf.DefaultGateway = nil
	path, _, _ := GenerateConfig(conf)
	content, _ := os.ReadFile(path)

	if !strings.Contains(string(content), "\ndhcp-option=3\n") {
		t.Errorf("dhcp-option=3 nue absente : sans elle dnsmasq annonce sa propre adresse comme routeur\n%s", content)
	}
}

func TestGenerateConfig_VxlanEmitsNoRouterOption(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	conf.DefaultGateway = nil

	path, _, _ := GenerateConfig(conf)
	content, _ := os.ReadFile(path)

	if !strings.Contains(string(content), "dhcp-option=121,") {
		t.Fatalf("dhcp-option=121 attendue pour un subnet vxlan :\n%s", content)
	}
	if strings.Contains(string(content), "dhcp-option=3,") {
		t.Errorf("un subnet vxlan est privé : aucune route par défaut ne doit être émise\n%s", content)
	}
}

func TestGenerateConfig_ContainsVPCRoute(t *testing.T) {
	routes := route121(t, confLines(t, newConf(t, "192.168.1.0/29")))
	if !strings.Contains(routes, "10.0.0.0/16,192.168.1.254") {
		t.Errorf("route VPC absente, ou next-hop autre que l'interface_ip du subnet :\n%s", routes)
	}
}

func TestGenerateConfig_NoVPCRoute(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	conf.VPCRoute = nil

	routes := route121(t, confLines(t, conf))
	if strings.Contains(routes, "10.0.0.0/16") {
		t.Errorf("route VPC présente alors que VPCRoute=nil :\n%s", routes)
	}
}

func TestGenerateConfig_ContainsDhcpRange(t *testing.T) {
	_, network, _ := net.ParseCIDR("10.10.0.0/24")
	conf := Config{
		Network:     network,
		InterfaceIP: net.ParseIP("10.10.0.1").To4(),
		Name:        "vpc1",
		ConfDir:     t.TempDir(),
	}
	path, _, _ := GenerateConfig(conf)
	content, _ := os.ReadFile(path)

	if !strings.Contains(string(content), "dhcp-range=10.10.0.0,static,255.255.255.0,12h") {
		t.Errorf("dhcp-range absent ou incorrect :\n%s", content)
	}
}

func TestGenerateConfig_OneHostEntryPerIP(t *testing.T) {
	// /29 = réseau + broadcast + 6 hôtes → 8 adresses
	conf := newConf(t, "10.0.0.0/29")
	path, _, _ := GenerateConfig(conf)
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
	path, _, _ := GenerateConfig(conf)
	content, _ := os.ReadFile(path)

	if !strings.Contains(string(content), "00:22:33:") {
		t.Errorf("préfixe MAC 00:22:33: absent :\n%s", content)
	}
}

func TestGenerateConfig_CreatesConfDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sous", "dossier")
	_, network, _ := net.ParseCIDR("10.0.0.0/30")
	conf := Config{
		Network:     network,
		InterfaceIP: net.ParseIP("10.0.0.1").To4(),
		Name:        "net",
		ConfDir:     dir,
	}
	if _, _, err := GenerateConfig(conf); err != nil {
		t.Fatalf("GenerateConfig devrait créer les répertoires manquants : %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("répertoire %q non créé", dir)
	}
}

// --- option 121 : routes classless ---

func confLines(t *testing.T, c Config) string {
	t.Helper()
	path, _, err := GenerateConfig(c)
	if err != nil {
		t.Fatalf("GenerateConfig : %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lecture : %v", err)
	}
	return string(content)
}

func route121(t *testing.T, content string) string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "dhcp-option=121,") {
			return strings.TrimPrefix(line, "dhcp-option=121,")
		}
	}
	t.Fatalf("aucune dhcp-option=121 dans :\n%s", content)
	return ""
}

func TestGenerateConfig_AlwaysRoutesToMetadata(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	conf.VPCRoute = nil
	conf.DefaultGateway = nil

	routes := route121(t, confLines(t, conf))
	if !strings.Contains(routes, "169.254.169.254/32,192.168.1.254") {
		t.Errorf("sans route vers le serveur de métadonnées, cloud-init échoue et la VM n'est pas provisionnée :\n%s", routes)
	}
}

func TestGenerateConfig_MetadataRouteEvenInBridgeMode(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	conf.VPCRoute = nil

	routes := route121(t, confLines(t, conf))
	if !strings.Contains(routes, "169.254.169.254/32") {
		t.Errorf("le mode bridge a besoin de la même route :\n%s", routes)
	}
}

func TestGenerateConfig_DefaultRouteAlsoInOption121(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	conf.VPCRoute = nil

	routes := route121(t, confLines(t, conf))
	if !strings.Contains(routes, "0.0.0.0/0,192.168.1.1") {
		t.Errorf("RFC 3442 : un client qui lit l'option 121 ignore l'option 3, la route par défaut doit donc figurer dans la 121 :\n%s", routes)
	}
}

func TestGenerateConfig_NoDefaultRouteMeansNoCatchAllInOption121(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	conf.DefaultGateway = nil

	routes := route121(t, confLines(t, conf))
	if strings.Contains(routes, "0.0.0.0/0") {
		t.Errorf("aucune route par défaut demandée, la 121 ne doit pas en contenir :\n%s", routes)
	}
}

func TestGenerateConfig_MissingInterfaceIPIsAnError(t *testing.T) {
	conf := newConf(t, "192.168.1.0/29")
	conf.InterfaceIP = nil

	if _, _, err := GenerateConfig(conf); err == nil {
		t.Error("sans interface_ip aucune route metadata n'est possible : il faut échouer, pas écrire une conf muette")
	}
}

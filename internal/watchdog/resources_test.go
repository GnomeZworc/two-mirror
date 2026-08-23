package watchdog

import (
	"reflect"
	"strings"
	"testing"
)

func TestResourceNames_ExtraitDepuisLesClesState(t *testing.T) {
	pairs := map[string]string{
		"vpc/vp-admin/state": "running",
		"vpc/vp-lab/state":   "creating",
	}
	want := []string{"vp-admin", "vp-lab"}
	if got := resourceNames(pairs, prefixVPC); !reflect.DeepEqual(got, want) {
		t.Errorf("resourceNames = %v, attendu %v", got, want)
	}
}

func TestResourceNames_IgnoreLesAutresCles(t *testing.T) {
	pairs := map[string]string{
		"subnet/br-000042/state":       "running",
		"subnet/br-000042/vpc":         "vp-admin",
		"subnet/br-000042/cidr":        "10.0.0.0/24",
		"subnet/br-000042/local_iface": "br-000042",
	}
	want := []string{"br-000042"}
	if got := resourceNames(pairs, prefixSubnet); !reflect.DeepEqual(got, want) {
		t.Errorf("resourceNames = %v, attendu %v", got, want)
	}
}

func TestResourceNames_IgnoreLesClesImbriquees(t *testing.T) {
	pairs := map[string]string{
		"vm/i-test1/state":          "running",
		"vm/i-test1/disk/vda/state": "attached",
		"vm/i-test1/disk/vdb/state": "attached",
		"vm/i-test1/dhcp/10.0.0.5":  "00:22:33:44:55:66",
	}
	want := []string{"i-test1"}
	if got := resourceNames(pairs, prefixVM); !reflect.DeepEqual(got, want) {
		t.Errorf("resourceNames = %v, attendu %v", got, want)
	}
}

func TestResourceNames_Trie(t *testing.T) {
	pairs := map[string]string{
		"vpc/vp-c/state": "running",
		"vpc/vp-a/state": "running",
		"vpc/vp-b/state": "running",
	}
	want := []string{"vp-a", "vp-b", "vp-c"}
	for i := range 20 {
		if got := resourceNames(pairs, prefixVPC); !reflect.DeepEqual(got, want) {
			t.Fatalf("itération %d : resourceNames = %v, attendu %v", i, got, want)
		}
	}
}

func TestResourceNames_MapVide(t *testing.T) {
	if got := resourceNames(map[string]string{}, prefixVPC); len(got) != 0 {
		t.Errorf("resourceNames = %v, attendu vide", got)
	}
}

func TestResourceNames_PrefixeNonCorrespondant(t *testing.T) {
	pairs := map[string]string{"vm/i-test1/state": "running"}
	if got := resourceNames(pairs, prefixVPC); len(got) != 0 {
		t.Errorf("resourceNames = %v, attendu vide", got)
	}
}

func TestLinkProblem_InterfaceInexistante(t *testing.T) {
	p := linkProblem("interface-qui-nexiste-pas-42")
	if p == "" {
		t.Fatal("linkProblem devrait signaler un problème pour une interface inexistante")
	}
	if !strings.Contains(p, "interface-qui-nexiste-pas-42") {
		t.Errorf("le message %q devrait nommer l'interface", p)
	}
}

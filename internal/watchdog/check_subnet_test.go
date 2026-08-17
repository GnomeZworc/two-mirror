package watchdog

import (
	"errors"
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/internal/state"

	"github.com/dgraph-io/badger/v4"
)

func seedSubnet(t *testing.T, db *badger.DB, name, vpc, mode string) {
	t.Helper()
	seedResource(t, db, prefixSubnet, name, state.Running)
	seedKV(t, db, prefixSubnet+name+"/vpc", vpc)
	seedKV(t, db, prefixSubnet+name+"/mode", mode)
}

func TestSubnetIfaceNames_NomStandard(t *testing.T) {
	hostVeth, nsVeth, bridge, err := subnetIfaceNames("br-000042")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if hostVeth != "v-000042-e" || nsVeth != "v-000042-i" || bridge != "br-000042" {
		t.Errorf("obtenu (%q, %q, %q), attendu (v-000042-e, v-000042-i, br-000042)", hostVeth, nsVeth, bridge)
	}
}

func TestSubnetIfaceNames_NomSansTiret(t *testing.T) {
	if _, _, _, err := subnetIfaceNames("subnet"); err == nil {
		t.Fatal("un nom sans tiret devrait produire une erreur, pas une panique")
	}
}

func TestSubnetIfaceNames_TiretFinal(t *testing.T) {
	if _, _, _, err := subnetIfaceNames("br-"); err == nil {
		t.Fatal("un identifiant vide devrait produire une erreur")
	}
}

func TestDnsmasqName(t *testing.T) {
	if got := dnsmasqName("vp-admin", "br-000000"); got != "vp-admin_br-000000" {
		t.Errorf("dnsmasqName = %q, attendu vp-admin_br-000000", got)
	}
}

func TestCheckSubnets_BaseVide(t *testing.T) {
	db := newTestDB(t)
	r := &recorder{}

	if err := CheckSubnets(db, newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("aucune notification attendue, obtenu %v", r.calls)
	}
}

func TestCheckSubnets_IgnoreLesEtatsNonRunning(t *testing.T) {
	db := newTestDB(t)
	for _, s := range []state.State{state.Creating, state.Deleting, state.Error, state.Deleted} {
		seedResource(t, db, prefixSubnet, "br-"+string(s), s)
	}
	r := &recorder{}

	if err := CheckSubnets(db, newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("aucune notification attendue, obtenu %v", r.calls)
	}
}

func TestCheckSubnets_VPCManquantEnBase(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixSubnet, "br-000042", state.Running)
	r := &recorder{}

	if err := CheckSubnets(db, newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	got := r.forName("br-000042")
	if len(got) != 1 {
		t.Fatalf("attendu 1 notification, obtenu %d : %v", len(got), got)
	}
	if !strings.Contains(got[0].problem, "vpc unreadable") {
		t.Errorf("problem = %q, devrait porter sur le vpc", got[0].problem)
	}
}

func TestCheckSubnets_ModeManquantEnBase(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixSubnet, "br-000042", state.Running)
	seedKV(t, db, prefixSubnet+"br-000042/vpc", "vp-admin")
	r := &recorder{}

	if err := CheckSubnets(db, newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("mode unreadable") {
		t.Errorf("devrait signaler un mode illisible, obtenu %v", r.calls)
	}
}

func TestCheckSubnets_ModeInconnu(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "br-000042", "vp-admin", "macvlan")
	r := &recorder{}

	if err := CheckSubnets(db, newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining(`unknown mode "macvlan"`) {
		t.Errorf("devrait signaler un mode inconnu, obtenu %v", r.calls)
	}
}

func TestCheckSubnets_ModeBridgeNeVerifiePasDeVxlan(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "br-000042", "vp-admin", modeBridge)
	r := &recorder{}

	if err := CheckSubnets(db, newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	for _, c := range r.calls {
		if strings.Contains(c.problem, "vxlan") {
			t.Errorf("un subnet en mode bridge ne doit rien signaler sur vxlan: %q", c.problem)
		}
		if strings.Contains(c.problem, "(host)") {
			t.Errorf("un subnet en mode bridge n'a pas de bridge host: %q", c.problem)
		}
	}
}

func TestCheckSubnets_ModeVxlanSansVxlanID(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "br-000042", "vp-admin", modeVxlan)
	r := &recorder{}

	if err := CheckSubnets(db, newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("vxlan_id unreadable") {
		t.Errorf("devrait signaler un vxlan_id illisible, obtenu %v", r.calls)
	}
}

func TestCheckSubnets_ModeVxlanVxlanIDInvalide(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "br-000042", "vp-admin", modeVxlan)
	seedKV(t, db, prefixSubnet+"br-000042/vxlan_id", "pas-un-nombre")
	r := &recorder{}

	if err := CheckSubnets(db, newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("invalid vxlan_id") {
		t.Errorf("devrait signaler un vxlan_id invalide, obtenu %v", r.calls)
	}
}

func TestCheckSubnets_ModeVxlanVerifieLInterfaceVxlan(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "br-000042", "vp-admin", modeVxlan)
	seedKV(t, db, prefixSubnet+"br-000042/vxlan_id", "42")
	r := &recorder{}

	if err := CheckSubnets(db, newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("vxlan-42") {
		t.Errorf("devrait signaler l'interface vxlan-42, obtenu %v", r.calls)
	}
}

func TestCheckSubnets_ConfigDnsmasqAbsente(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "br-000042", "vp-admin", modeBridge)
	r := &recorder{}

	if err := CheckSubnets(db, newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("dnsmasq config missing") {
		t.Errorf("devrait signaler la config dnsmasq absente, obtenu %v", r.calls)
	}
	if !r.hasProblemContaining("vp-admin_br-000042.conf") {
		t.Errorf("devrait nommer le fichier attendu, obtenu %v", r.calls)
	}
}

func TestCheckSubnets_UnitDnsmasqInterrogee(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "br-000042", "vp-admin", modeBridge)
	u := newFakeUnits().active("dnsmasq@vp-admin_br-000042.service")
	r := &recorder{}

	if err := CheckSubnets(db, u, r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if len(u.asked) != 1 || u.asked[0] != "dnsmasq@vp-admin_br-000042.service" {
		t.Errorf("units interrogées = %v, attendu [dnsmasq@vp-admin_br-000042.service]", u.asked)
	}
	if r.hasProblemContaining("unit ") {
		t.Errorf("une unit active ne doit rien signaler, obtenu %v", r.calls)
	}
}

func TestCheckSubnets_UnitDnsmasqInactive(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "br-000042", "vp-admin", modeBridge)
	u := newFakeUnits().inactive("dnsmasq@vp-admin_br-000042.service", "failed")
	r := &recorder{}

	if err := CheckSubnets(db, u, r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("unit dnsmasq@vp-admin_br-000042.service inactive (failed)") {
		t.Errorf("devrait signaler l'unit inactive, obtenu %v", r.calls)
	}
}

func TestCheckSubnets_UnitIllisible(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "br-000042", "vp-admin", modeBridge)
	u := newFakeUnits().failing("dnsmasq@vp-admin_br-000042.service", errors.New("dbus indisponible"))
	r := &recorder{}

	if err := CheckSubnets(db, u, r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("unit dnsmasq@vp-admin_br-000042.service unreadable") {
		t.Errorf("devrait signaler l'unit illisible, obtenu %v", r.calls)
	}
}

func TestCheckSubnets_SansUnitCheckerPasDeVerificationDUnit(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "br-000042", "vp-admin", modeBridge)
	r := &recorder{}

	if err := CheckSubnets(db, nil, r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if r.hasProblemContaining("unit ") {
		t.Errorf("sans unitChecker, aucune notification d'unit attendue, obtenu %v", r.calls)
	}
}

func TestCheckSubnets_EtatCorrompuNInterrompPasLaBoucle(t *testing.T) {
	db := newTestDB(t)
	seedKV(t, db, prefixSubnet+"br-corrompu"+stateSuffix, "n_importe_quoi")
	seedSubnet(t, db, "br-000042", "vp-admin", modeBridge)
	r := &recorder{}

	if err := CheckSubnets(db, newFakeUnits(), r); err != nil {
		t.Fatalf("un état corrompu ne doit pas faire échouer CheckSubnets: %v", err)
	}

	if !strings.Contains(strings.Join(problems(r.forName("br-corrompu")), " "), "state unreadable") {
		t.Errorf("devrait signaler l'état corrompu, obtenu %v", r.calls)
	}
	if len(r.forName("br-000042")) == 0 {
		t.Error("une clé corrompue ne doit pas empêcher l'examen des subnets suivants")
	}
}

func problems(ns []notification) []string {
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, n.problem)
	}
	return out
}

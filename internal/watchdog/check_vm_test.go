package watchdog

import (
	"strings"
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/state"

	"github.com/dgraph-io/badger/v4"
)

func testCfg(t *testing.T) *configuration.Config {
	t.Helper()
	cfg := &configuration.Config{}
	cfg.QEMU.QMPDir = t.TempDir()
	return cfg
}

func seedVM(t *testing.T, db *badger.DB, name, subnetName, vpc, tapID string) {
	t.Helper()
	seedResource(t, db, prefixVM, name, state.Running)
	seedKV(t, db, prefixVM+name+"/subnet", subnetName)
	seedKV(t, db, prefixSubnet+subnetName+"/vpc", vpc)
	if tapID != "" {
		seedKV(t, db, prefixVM+name+"/tap_id", tapID)
	}
}

func TestTapName(t *testing.T) {
	if got := tapName(12345678); got != "tap12345678" {
		t.Errorf("tapName = %q, attendu tap12345678", got)
	}
}

func TestCheckVMs_ConfigNil(t *testing.T) {
	db := newTestDB(t)
	r := &recorder{}

	err := CheckVMs(db, nil, newFakeUnits(), r)
	if err == nil {
		t.Fatal("une config nil doit produire une erreur, pas une panique")
	}
	if len(r.calls) != 0 {
		t.Errorf("aucune notification attendue, obtenu %v", r.calls)
	}
}

func TestCheckVMs_BaseVide(t *testing.T) {
	db := newTestDB(t)
	r := &recorder{}

	if err := CheckVMs(db, testCfg(t), newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("aucune notification attendue, obtenu %v", r.calls)
	}
}

func TestCheckVMs_IgnoreLesEtatsNonRunning(t *testing.T) {
	db := newTestDB(t)
	for _, s := range []state.State{state.Creating, state.Deleting, state.Error, state.Deleted} {
		seedResource(t, db, prefixVM, "i-"+string(s), s)
	}
	r := &recorder{}

	if err := CheckVMs(db, testCfg(t), newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("aucune notification attendue, obtenu %v", r.calls)
	}
}

func TestCheckVMs_SubnetManquantEnBase(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVM, "i-test1", state.Running)
	r := &recorder{}

	if err := CheckVMs(db, testCfg(t), newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	got := r.forName("i-test1")
	if len(got) != 1 {
		t.Fatalf("attendu 1 notification, obtenu %d : %v", len(got), got)
	}
	if !strings.Contains(got[0].problem, "subnet illisible") {
		t.Errorf("problem = %q, devrait porter sur le subnet", got[0].problem)
	}
}

func TestCheckVMs_VPCDuSubnetManquant(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVM, "i-test1", state.Running)
	seedKV(t, db, prefixVM+"i-test1/subnet", "br-000042")
	r := &recorder{}

	if err := CheckVMs(db, testCfg(t), newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("vpc du subnet br-000042 illisible") {
		t.Errorf("devrait signaler le vpc introuvable, obtenu %v", r.calls)
	}
}

func TestCheckVMs_TapIDManquant(t *testing.T) {
	db := newTestDB(t)
	seedVM(t, db, "i-test1", "br-000042", "vp-admin", "")
	r := &recorder{}

	if err := CheckVMs(db, testCfg(t), newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("tap_id illisible") {
		t.Errorf("devrait signaler un tap_id illisible, obtenu %v", r.calls)
	}
}

func TestCheckVMs_TapIDInvalide(t *testing.T) {
	db := newTestDB(t)
	seedVM(t, db, "i-test1", "br-000042", "vp-admin", "pas-un-nombre")
	r := &recorder{}

	if err := CheckVMs(db, testCfg(t), newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("tap_id invalide") {
		t.Errorf("devrait signaler un tap_id invalide, obtenu %v", r.calls)
	}
}

func TestCheckVMs_QemuNeRepondPas(t *testing.T) {
	db := newTestDB(t)
	seedVM(t, db, "i-test1", "br-000042", "vp-admin", "12345678")
	cfg := testCfg(t)
	r := &recorder{}

	if err := CheckVMs(db, cfg, newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("qemu ne répond pas") {
		t.Errorf("devrait signaler que qemu ne répond pas, obtenu %v", r.calls)
	}
	if !r.hasProblemContaining("i-test1.sock") {
		t.Errorf("devrait nommer la socket attendue, obtenu %v", r.calls)
	}
}

func TestCheckVMs_UnitsInterrogees(t *testing.T) {
	db := newTestDB(t)
	seedVM(t, db, "i-test1", "br-000042", "vp-admin", "12345678")
	u := newFakeUnits().active("metadata@i-test1.service").active("two-vm-i-test1.scope")
	r := &recorder{}

	if err := CheckVMs(db, testCfg(t), u, r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	want := map[string]bool{"metadata@i-test1.service": false, "two-vm-i-test1.scope": false}
	for _, asked := range u.asked {
		if _, ok := want[asked]; !ok {
			t.Errorf("unit inattendue interrogée: %q", asked)
			continue
		}
		want[asked] = true
	}
	for unit, seen := range want {
		if !seen {
			t.Errorf("unit %q non interrogée (interrogées: %v)", unit, u.asked)
		}
	}
	if r.hasProblemContaining("unit ") {
		t.Errorf("des units actives ne doivent rien signaler, obtenu %v", r.calls)
	}
}

func TestCheckVMs_MetadataInactive(t *testing.T) {
	db := newTestDB(t)
	seedVM(t, db, "i-test1", "br-000042", "vp-admin", "12345678")
	u := newFakeUnits().inactive("metadata@i-test1.service", "dead").active("two-vm-i-test1.scope")
	r := &recorder{}

	if err := CheckVMs(db, testCfg(t), u, r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("unit metadata@i-test1.service inactive (dead)") {
		t.Errorf("devrait signaler metadata inactive, obtenu %v", r.calls)
	}
}

func TestCheckVMs_ScopeAbsent(t *testing.T) {
	db := newTestDB(t)
	seedVM(t, db, "i-test1", "br-000042", "vp-admin", "12345678")
	u := newFakeUnits().active("metadata@i-test1.service")
	r := &recorder{}

	if err := CheckVMs(db, testCfg(t), u, r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	if !r.hasProblemContaining("unit two-vm-i-test1.scope inactive") {
		t.Errorf("devrait signaler le scope absent, obtenu %v", r.calls)
	}
}

func TestCheckVMs_EtatCorrompuNInterrompPasLaBoucle(t *testing.T) {
	db := newTestDB(t)
	seedKV(t, db, prefixVM+"i-corrompu"+stateSuffix, "n_importe_quoi")
	seedVM(t, db, "i-test1", "br-000042", "vp-admin", "12345678")
	r := &recorder{}

	if err := CheckVMs(db, testCfg(t), newFakeUnits(), r); err != nil {
		t.Fatalf("un état corrompu ne doit pas faire échouer CheckVMs: %v", err)
	}

	if !strings.Contains(strings.Join(problems(r.forName("i-corrompu")), " "), "état illisible") {
		t.Errorf("devrait signaler l'état corrompu, obtenu %v", r.calls)
	}
	if len(r.forName("i-test1")) == 0 {
		t.Error("une clé corrompue ne doit pas empêcher l'examen des VMs suivantes")
	}
}

func TestCheckVMs_ClesDeDisqueNeCreentPasDeFausseVM(t *testing.T) {
	db := newTestDB(t)
	seedVM(t, db, "i-test1", "br-000042", "vp-admin", "12345678")
	seedKV(t, db, prefixVM+"i-test1/disk/vda/state", "attached")
	r := &recorder{}

	if err := CheckVMs(db, testCfg(t), newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	for _, c := range r.calls {
		if c.name != "i-test1" {
			t.Errorf("ressource inattendue signalée: %q", c.name)
		}
	}
}

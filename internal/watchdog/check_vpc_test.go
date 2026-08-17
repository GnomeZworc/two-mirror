package watchdog

import (
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func TestVPCIfaceNames_NomStandard(t *testing.T) {
	host, ns, err := vpcIfaceNames("vp-admin")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if host != "vp-admin-e" || ns != "vp-admin-i" {
		t.Errorf("obtenu (%q, %q), attendu (vp-admin-e, vp-admin-i)", host, ns)
	}
}

func TestVPCIfaceNames_IdentifiantNumerique(t *testing.T) {
	host, ns, err := vpcIfaceNames("vpc-000003")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if host != "vp-000003-e" || ns != "vp-000003-i" {
		t.Errorf("obtenu (%q, %q), attendu (vp-000003-e, vp-000003-i)", host, ns)
	}
}

func TestVPCIfaceNames_PlusieursTirets(t *testing.T) {
	host, _, err := vpcIfaceNames("vp-admin-prod")
	if err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if host != "vp-admin-prod-e" {
		t.Errorf("obtenu %q, attendu vp-admin-prod-e", host)
	}
}

func TestVPCIfaceNames_NomSansTiret(t *testing.T) {
	if _, _, err := vpcIfaceNames("admin"); err == nil {
		t.Fatal("un nom sans tiret devrait produire une erreur, pas une panique")
	}
}

func TestVPCIfaceNames_TiretFinal(t *testing.T) {
	if _, _, err := vpcIfaceNames("vp-"); err == nil {
		t.Fatal("un identifiant vide devrait produire une erreur")
	}
}

func TestVPCIfaceNames_NomVide(t *testing.T) {
	if _, _, err := vpcIfaceNames(""); err == nil {
		t.Fatal("un nom vide devrait produire une erreur")
	}
}

func TestCheckVPCs_BaseVide(t *testing.T) {
	db := newTestDB(t)
	r := &recorder{}

	if err := CheckVPCs(db, r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("aucune notification attendue, obtenu %v", r.calls)
	}
}

func TestCheckVPCs_IgnoreLesEtatsNonRunning(t *testing.T) {
	db := newTestDB(t)
	for _, s := range []state.State{state.Creating, state.Deleting, state.Error, state.Deleted} {
		seedResource(t, db, prefixVPC, "vp-"+string(s), s)
	}
	r := &recorder{}

	if err := CheckVPCs(db, r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("aucune notification attendue pour des états non-running, obtenu %v", r.calls)
	}
}

func TestCheckVPCs_SignaleUnVPCRunningAbsent(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "vp-fantome", state.Running)
	r := &recorder{}

	if err := CheckVPCs(db, r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	got := r.forName("vp-fantome")
	if len(got) == 0 {
		t.Fatal("un VPC running inexistant sur le système doit être signalé")
	}
	for _, c := range got {
		if c.kind != kindVPC {
			t.Errorf("kind = %q, attendu %q", c.kind, kindVPC)
		}
		if c.problem == "" {
			t.Error("problem ne doit pas être vide")
		}
	}
}

func TestCheckVPCs_EtatCorrompu(t *testing.T) {
	db := newTestDB(t)
	if err := kv.AddInDB(db, prefixVPC+"vp-corrompu"+stateSuffix, "n_importe_quoi"); err != nil {
		t.Fatalf("préparation: %v", err)
	}
	seedResource(t, db, prefixVPC, "vp-suivant", state.Running)
	r := &recorder{}

	if err := CheckVPCs(db, r); err != nil {
		t.Fatalf("un état corrompu ne doit pas faire échouer CheckVPCs: %v", err)
	}

	got := r.forName("vp-corrompu")
	if len(got) != 1 {
		t.Fatalf("attendu 1 notification pour l'état corrompu, obtenu %d", len(got))
	}
	if !strings.Contains(got[0].problem, "state unreadable") {
		t.Errorf("problem = %q, devrait mentionner un état illisible", got[0].problem)
	}
	if len(r.forName("vp-suivant")) == 0 {
		t.Error("une clé corrompue ne doit pas empêcher l'examen des VPC suivants")
	}
}

func TestCheckVPCs_NomIndeductibleNePaniquePas(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "admin", state.Running)
	r := &recorder{}

	if err := CheckVPCs(db, r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}

	got := r.forName("admin")
	if len(got) != 1 {
		t.Fatalf("attendu 1 notification, obtenu %d : %v", len(got), got)
	}
	if !strings.Contains(got[0].problem, "cannot be derived") {
		t.Errorf("problem = %q, devrait porter sur les interfaces indéductibles", got[0].problem)
	}
}

func TestCheckVPCs_OrdreDeterministe(t *testing.T) {
	db := newTestDB(t)
	for _, name := range []string{"vp-c", "vp-a", "vp-b"} {
		seedResource(t, db, prefixVPC, name, state.Running)
	}

	var first []string
	for i := range 5 {
		r := &recorder{}
		if err := CheckVPCs(db, r); err != nil {
			t.Fatalf("erreur inattendue: %v", err)
		}
		var order []string
		for _, c := range r.calls {
			if len(order) == 0 || order[len(order)-1] != c.name {
				order = append(order, c.name)
			}
		}
		if i == 0 {
			first = order
			continue
		}
		if strings.Join(order, ",") != strings.Join(first, ",") {
			t.Fatalf("itération %d : ordre %v, attendu %v", i, order, first)
		}
	}
}

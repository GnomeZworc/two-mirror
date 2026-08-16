package state

import (
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

// newTestDB ouvre une base BadgerDB dans un répertoire temporaire.
// La base est fermée automatiquement en fin de test.
func newTestDB(t *testing.T) *badger.DB {
	t.Helper()
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	return db
}

// --- Parse ---

func TestParse_ValidStates(t *testing.T) {
	for _, s := range All() {
		got, err := Parse(string(s))
		if err != nil {
			t.Errorf("Parse(%q) a échoué : %v", s, err)
		}
		if got != s {
			t.Errorf("Parse(%q) = %q, attendu %q", s, got, s)
		}
	}
}

func TestParse_UnknownState(t *testing.T) {
	// "created" et "started" sont les anciens états : ils ne doivent plus être
	// acceptés une fois la migration passée.
	for _, raw := range []string{"created", "started", "stopping", "", "RUNNING"} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) devrait échouer", raw)
		}
	}
}

// --- All ---

func TestAll_ContainsEveryState(t *testing.T) {
	want := map[State]bool{Creating: false, Running: false, Error: false, Deleting: false, Deleted: false}
	for _, s := range All() {
		if _, ok := want[s]; !ok {
			t.Errorf("All() retourne un état inattendu %q", s)
		}
		want[s] = true
	}
	for s, seen := range want {
		if !seen {
			t.Errorf("All() ne contient pas %q", s)
		}
	}
}

// --- CanDelete ---

func TestCanDelete(t *testing.T) {
	cases := map[State]bool{
		Running:  true,
		Error:    true,
		Creating: false,
		Deleting: false,
		Deleted:  false,
	}
	for s, want := range cases {
		if got := CanDelete(s); got != want {
			t.Errorf("CanDelete(%q) = %v, attendu %v", s, got, want)
		}
	}
}

// --- IsTransient ---

func TestIsTransient(t *testing.T) {
	cases := map[State]bool{
		Creating: true,
		Deleting: true,
		Running:  false,
		Error:    false,
		Deleted:  false,
	}
	for s, want := range cases {
		if got := IsTransient(s); got != want {
			t.Errorf("IsTransient(%q) = %v, attendu %v", s, got, want)
		}
	}
}

// --- Get / Set ---

func TestSetGet_RoundTrip(t *testing.T) {
	db := newTestDB(t)

	if err := Set(db, "vpc/vpc-1", Creating); err != nil {
		t.Fatalf("Set a échoué : %v", err)
	}
	got, err := Get(db, "vpc/vpc-1")
	if err != nil {
		t.Fatalf("Get a échoué : %v", err)
	}
	if got != Creating {
		t.Errorf("Get = %q, attendu %q", got, Creating)
	}

	if err := Set(db, "vpc/vpc-1", Running); err != nil {
		t.Fatalf("Set a échoué : %v", err)
	}
	got, err = Get(db, "vpc/vpc-1")
	if err != nil {
		t.Fatalf("Get a échoué : %v", err)
	}
	if got != Running {
		t.Errorf("Get après écrasement = %q, attendu %q", got, Running)
	}
}

func TestSet_WritesStateSuffix(t *testing.T) {
	db := newTestDB(t)

	if err := Set(db, "vm/vm-1", Running); err != nil {
		t.Fatalf("Set a échoué : %v", err)
	}
	raw, err := kv.GetFromDB(db, "vm/vm-1/state")
	if err != nil {
		t.Fatalf("la clé vm/vm-1/state devrait exister : %v", err)
	}
	if raw != "running" {
		t.Errorf("valeur en DB = %q, attendu \"running\"", raw)
	}
}

func TestSet_InvalidState(t *testing.T) {
	db := newTestDB(t)

	if err := Set(db, "vpc/vpc-1", State("created")); err == nil {
		t.Error("Set devrait refuser un état inconnu")
	}
	if _, err := kv.GetFromDB(db, "vpc/vpc-1/state"); err == nil {
		t.Error("aucune clé ne devrait être écrite pour un état invalide")
	}
}

func TestGet_MissingKey(t *testing.T) {
	db := newTestDB(t)

	if _, err := Get(db, "vpc/inconnu"); err == nil {
		t.Error("Get devrait échouer sur une ressource absente")
	}
}

func TestGet_CorruptedValue(t *testing.T) {
	db := newTestDB(t)

	// Valeur écrite hors du package (ancien état, corruption) : Get doit
	// remonter l'erreur plutôt que de retourner un State silencieusement vide.
	if err := kv.AddInDB(db, "subnet/sn-1/state", "created"); err != nil {
		t.Fatalf("préparation du test : %v", err)
	}
	if _, err := Get(db, "subnet/sn-1"); err == nil {
		t.Error("Get devrait échouer sur une valeur non reconnue")
	}
}

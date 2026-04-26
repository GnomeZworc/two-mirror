package kv

import (
	"errors"
	"testing"

	"github.com/dgraph-io/badger/v4"
)

// newTestDB ouvre une base BadgerDB dans un répertoire temporaire.
// La base est fermée automatiquement en fin de test.
func newTestDB(t *testing.T) *badger.DB {
	t.Helper()
	db := InitDB(Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	return db
}

// --- InitDB ---

func TestInitDB_ValidPath(t *testing.T) {
	db := newTestDB(t)
	if db == nil {
		t.Fatal("InitDB devrait retourner une DB non-nil")
	}
}

func TestInitDB_InvalidPath_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("InitDB avec un chemin invalide devrait paniquer")
		}
	}()
	InitDB(Config{Path: "/chemin/inexistant/absolu"}, false)
}

// --- AddInDB ---

func TestAddInDB_NewKey(t *testing.T) {
	db := newTestDB(t)
	if err := AddInDB(db, "vpc/test", "valeur"); err != nil {
		t.Fatalf("AddInDB a échoué : %v", err)
	}
}

func TestAddInDB_OverwriteExistingKey(t *testing.T) {
	db := newTestDB(t)
	AddInDB(db, "vpc/test", "premiere")
	if err := AddInDB(db, "vpc/test", "deuxieme"); err != nil {
		t.Fatalf("AddInDB (écrasement) a échoué : %v", err)
	}

	val, _ := GetFromDB(db, "vpc/test")
	if val != "deuxieme" {
		t.Errorf("valeur attendue %q, obtenu %q", "deuxieme", val)
	}
}

// --- GetFromDB ---

func TestGetFromDB_ExistingKey(t *testing.T) {
	db := newTestDB(t)
	AddInDB(db, "vpc/foo", "bar")

	val, err := GetFromDB(db, "vpc/foo")
	if err != nil {
		t.Fatalf("GetFromDB a échoué : %v", err)
	}
	if val != "bar" {
		t.Errorf("valeur attendue %q, obtenu %q", "bar", val)
	}
}

func TestGetFromDB_MissingKey(t *testing.T) {
	db := newTestDB(t)

	_, err := GetFromDB(db, "inexistant")
	if !errors.Is(err, badger.ErrKeyNotFound) {
		t.Errorf("erreur attendue ErrKeyNotFound, obtenu : %v", err)
	}
}

func TestGetFromDB_EmptyValue(t *testing.T) {
	db := newTestDB(t)
	AddInDB(db, "vpc/vide", "")

	val, err := GetFromDB(db, "vpc/vide")
	if err != nil {
		t.Fatalf("GetFromDB a échoué : %v", err)
	}
	if val != "" {
		t.Errorf("valeur attendue vide, obtenu %q", val)
	}
}

// --- DeleteInDB ---

func TestDeleteInDB_SimpleKey(t *testing.T) {
	db := newTestDB(t)
	AddInDB(db, "vpc/a", "v")

	if err := DeleteInDB(db, "vpc/a"); err != nil {
		t.Fatalf("DeleteInDB a échoué : %v", err)
	}

	_, err := GetFromDB(db, "vpc/a")
	if !errors.Is(err, badger.ErrKeyNotFound) {
		t.Errorf("la clé devrait être supprimée, obtenu : %v", err)
	}
}

func TestDeleteInDB_WithSubkeys(t *testing.T) {
	db := newTestDB(t)
	// Clé parente + sous-clés (préfixe "vpc/net1/")
	AddInDB(db, "vpc/net1", "parent")
	AddInDB(db, "vpc/net1/ip", "10.0.0.1")
	AddInDB(db, "vpc/net1/gw", "10.0.0.254")

	if err := DeleteInDB(db, "vpc/net1"); err != nil {
		t.Fatalf("DeleteInDB a échoué : %v", err)
	}

	for _, key := range []string{"vpc/net1", "vpc/net1/ip", "vpc/net1/gw"} {
		_, err := GetFromDB(db, key)
		if !errors.Is(err, badger.ErrKeyNotFound) {
			t.Errorf("clé %q devrait être supprimée, obtenu : %v", key, err)
		}
	}
}

func TestDeleteInDB_DoesNotDeleteSiblings(t *testing.T) {
	db := newTestDB(t)
	AddInDB(db, "vpc/net1", "a")
	AddInDB(db, "vpc/net2", "b") // ne doit pas être supprimée

	DeleteInDB(db, "vpc/net1")

	val, err := GetFromDB(db, "vpc/net2")
	if err != nil {
		t.Fatalf("vpc/net2 ne devrait pas être supprimée : %v", err)
	}
	if val != "b" {
		t.Errorf("valeur attendue %q, obtenu %q", "b", val)
	}
}

func TestDeleteInDB_MissingKey(t *testing.T) {
	db := newTestDB(t)
	// Supprimer une clé inexistante ne doit pas crasher
	if err := DeleteInDB(db, "inexistant"); err != nil {
		t.Logf("DeleteInDB clé inexistante retourne : %v (non bloquant)", err)
	}
}

// --- ListByPrefix ---

func TestListByPrefix_MatchingKeys(t *testing.T) {
	db := newTestDB(t)
	AddInDB(db, "subnet/sn1/state", "created")
	AddInDB(db, "subnet/sn1/vpc", "vpc-1")
	AddInDB(db, "subnet/sn1/cidr", "10.0.0.0/24")

	entries, err := ListByPrefix(db, "subnet/sn1/")
	if err != nil {
		t.Fatalf("ListByPrefix a échoué : %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("attendu 3 entrées, obtenu %d", len(entries))
	}
	if entries["subnet/sn1/state"] != "created" {
		t.Errorf("valeur inattendue pour state : %q", entries["subnet/sn1/state"])
	}
	if entries["subnet/sn1/vpc"] != "vpc-1" {
		t.Errorf("valeur inattendue pour vpc : %q", entries["subnet/sn1/vpc"])
	}
}

func TestListByPrefix_NoMatch(t *testing.T) {
	db := newTestDB(t)
	AddInDB(db, "vpc/v1/state", "created")

	entries, err := ListByPrefix(db, "subnet/")
	if err != nil {
		t.Fatalf("ListByPrefix a échoué : %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("attendu 0 entrées, obtenu %d", len(entries))
	}
}

func TestListByPrefix_IsolatesPrefix(t *testing.T) {
	db := newTestDB(t)
	AddInDB(db, "subnet/sn1/state", "created")
	AddInDB(db, "subnet/sn2/state", "creating")
	AddInDB(db, "vpc/v1/state", "created")

	entries, err := ListByPrefix(db, "subnet/sn1/")
	if err != nil {
		t.Fatalf("ListByPrefix a échoué : %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("attendu 1 entrée, obtenu %d : %v", len(entries), entries)
	}
	if _, ok := entries["subnet/sn1/state"]; !ok {
		t.Error("subnet/sn1/state devrait être présent")
	}
}

func TestListByPrefix_EmptyDB(t *testing.T) {
	db := newTestDB(t)

	entries, err := ListByPrefix(db, "subnet/")
	if err != nil {
		t.Fatalf("ListByPrefix a échoué : %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("attendu 0 entrées sur DB vide, obtenu %d", len(entries))
	}
}

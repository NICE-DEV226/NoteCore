package preferences_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"notecore/backend/internal/preferences"
	"notecore/backend/internal/storage"
)

func newStore(t *testing.T) *preferences.Store {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatalf("ouverture base : %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return preferences.New(db)
}

// Un réglage non défini n'est pas une erreur : l'interface applique sa
// valeur par défaut.
func TestGetReglageAbsent(t *testing.T) {
	store := newStore(t)
	got, err := store.Get(context.Background(), "theme")
	if err != nil {
		t.Fatalf("lecture : %v", err)
	}
	if got != "" {
		t.Fatalf("réglage absent : attendu %q, obtenu %q", "", got)
	}
}

// Écrit puis relu : la préférence est bien conservée.
func TestSetPuisGet(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	if err := store.Set(ctx, "theme", "dark"); err != nil {
		t.Fatalf("écriture : %v", err)
	}
	got, err := store.Get(ctx, "theme")
	if err != nil {
		t.Fatalf("lecture : %v", err)
	}
	if got != "dark" {
		t.Fatalf("attendu %q, obtenu %q", "dark", got)
	}
}

// La réécriture d'une clé remplace la valeur au lieu de la dupliquer
// (la clé est la clé primaire de la table).
func TestSetEcrase(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	if err := store.Set(ctx, "theme", "dark"); err != nil {
		t.Fatalf("écriture 1 : %v", err)
	}
	if err := store.Set(ctx, "theme", "light"); err != nil {
		t.Fatalf("écriture 2 : %v", err)
	}
	all, err := store.All(ctx)
	if err != nil {
		t.Fatalf("lecture : %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("attendu 1 réglage, obtenu %d : %v", len(all), all)
	}
	if all["theme"] != "light" {
		t.Fatalf("attendu %q, obtenu %q", "light", all["theme"])
	}
}

// Les entrées hors bornes sont refusées plutôt qu'écrites.
func TestRefuseEntreesInvalides(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "   ", "x"); err == nil {
		t.Fatal("clé vide acceptée")
	}
	if err := store.Set(ctx, strings.Repeat("k", 65), "x"); err == nil {
		t.Fatal("clé trop longue acceptée")
	}
	if err := store.Set(ctx, "theme", strings.Repeat("v", 513)); err == nil {
		t.Fatal("valeur trop longue acceptée")
	}
	// Rien n'a été écrit.
	all, err := store.All(ctx)
	if err != nil {
		t.Fatalf("lecture : %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("réglages inattendus : %v", all)
	}
}

// Le réglage survit à la fermeture puis réouverture de la base — c'est
// l'exigence « conservée après redémarrage » (cahier des charges §3.6).
func TestReglagePersisteApresReouverture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	ctx := context.Background()

	db1, err := storage.Open(path)
	if err != nil {
		t.Fatalf("ouverture 1 : %v", err)
	}
	if err := preferences.New(db1).Set(ctx, "theme", "dark"); err != nil {
		t.Fatalf("écriture : %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("fermeture 1 : %v", err)
	}

	db2, err := storage.Open(path)
	if err != nil {
		t.Fatalf("ouverture 2 : %v", err)
	}
	defer db2.Close()

	got, err := preferences.New(db2).Get(ctx, "theme")
	if err != nil {
		t.Fatalf("lecture : %v", err)
	}
	if got != "dark" {
		t.Fatalf("attendu %q, obtenu %q", "dark", got)
	}
}

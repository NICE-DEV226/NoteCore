// Tests de la façade de transfert : la copie de sécurité prise avant une
// restauration doit être une base SQLite exploitable, écrite à côté de la
// base courante et sans droits superflus (ADR-004).
package bindings

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"notecore/backend/internal/notes"
	"notecore/backend/internal/storage"
)

func TestCopieDeSecuriteAvantRestauration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "notecore.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("ouverture de la base : %v", err)
	}
	defer db.Close()

	svc := notes.New(db)
	if _, err := svc.CreateNote(context.Background(), "avant restauration", ""); err != nil {
		t.Fatalf("création de note : %v", err)
	}

	core := New(svc, db)
	dest, err := core.safetyCopy()
	if err != nil {
		t.Fatalf("copie de sécurité : %v", err)
	}
	if got := filepath.Dir(dest); got != filepath.Dir(dbPath) {
		t.Errorf("copie écrite dans %q, attendu à côté de la base (%q)", got, filepath.Dir(dbPath))
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("copie absente : %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions de la copie = %o, attendu 600", perm)
	}

	// La copie doit rester une base lisible : c'est elle qu'on restaurera
	// si la restauration choisie s'avère malheureuse.
	copyDB, err := sql.Open("sqlite", "file:"+dest+"?mode=ro")
	if err != nil {
		t.Fatalf("ouverture de la copie : %v", err)
	}
	defer copyDB.Close()

	var count int
	if err := copyDB.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&count); err != nil {
		t.Fatalf("lecture de la copie : %v", err)
	}
	if count != 1 {
		t.Errorf("notes dans la copie = %d, attendu 1", count)
	}
}

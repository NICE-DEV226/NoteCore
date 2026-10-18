// Tests des migrations : une installation plus récente doit conserver
// les données de la précédente (§11, critère 12). La copie avant
// migration est le filet de sécurité qui rend un upgrade rattrapable.
package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openCopy(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("ouverture de la copie : %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func countNotes(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&count); err != nil {
		t.Fatalf("comptage des notes : %v", err)
	}
	return count
}

// Une base neuve n'écrit aucune copie : rien à conserver, rien à annoncer.
func TestOuvertureNeuveSansCopie(t *testing.T) {
	db, report, err := OpenWithReport(filepath.Join(t.TempDir(), "notecore.db"))
	if err != nil {
		t.Fatalf("ouverture : %v", err)
	}
	defer db.Close()

	if report.SafetyCopy != "" {
		t.Errorf("copie inutile sur une base neuve : %s", report.SafetyCopy)
	}
	if len(report.Applied) == 0 {
		t.Error("les migrations initiales devraient être appliquées")
	}
}

// Une base déjà à jour ne bouge plus : pas de copie à chaque lancement.
func TestReouvertureSansMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notecore.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("ouverture : %v", err)
	}
	db.Close()

	reopened, report, err := OpenWithReport(path)
	if err != nil {
		t.Fatalf("réouverture : %v", err)
	}
	defer reopened.Close()

	if report.SafetyCopy != "" {
		t.Errorf("copie écrite alors que rien n'a bougé : %s", report.SafetyCopy)
	}
	if len(report.Applied) != 0 {
		t.Errorf("migration appliquée deux fois : %v", report.Applied)
	}
}

// Simule une base laissée par une version plus ancienne de NoteCore
// (schéma 1 : ni pièces jointes ni cartes). La version courante doit la
// mettre à jour ET en conserver une copie exploitable, à côté d'elle.
func TestMigrationConserveLaBasePrecedente(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notecore.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("ouverture : %v", err)
	}
	// Contenu utilisateur présent avant l'upgrade.
	if _, err := db.Exec(
		`INSERT INTO notes (id, title, content, is_pinned, tags_text, created_at, updated_at)
		 VALUES ('n-1', 'note d avant', 'contenu', 0, '', ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("insertion : %v", err)
	}
	// Recule la base à l'état d'une installation antérieure.
	for _, table := range []string{"review_log", "cards", "attachments"} {
		if _, err := db.Exec(`DROP TABLE IF EXISTS ` + table); err != nil {
			t.Fatalf("retour arrière : %v", err)
		}
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version >= 2`); err != nil {
		t.Fatalf("retour arrière : %v", err)
	}
	db.Close()

	upgraded, report, err := OpenWithReport(path)
	if err != nil {
		t.Fatalf("upgrade : %v", err)
	}
	defer upgraded.Close()

	if report.SafetyCopy == "" {
		t.Fatal("aucune copie conservée avant la migration du schéma")
	}
	if len(report.Applied) == 0 {
		t.Error("aucune migration appliquée malgré un schéma antérieur")
	}

	// Les données de l'installation précédente sont intactes dans la
	// copie : c'est elle qu'on relirait si l'upgrade s'avérait défaillant.
	copied := openCopy(t, report.SafetyCopy)
	if got := countNotes(t, copied); got != 1 {
		t.Errorf("notes dans la copie = %d, attendu 1", got)
	}
	var title string
	if err := copied.QueryRow(`SELECT title FROM notes WHERE id = 'n-1'`).Scan(&title); err != nil {
		t.Fatalf("lecture de la copie : %v", err)
	}
	if title != "note d avant" {
		t.Errorf("titre dans la copie = %q", title)
	}

	// La copie est en 0600, comme tout fichier de données.
	info, err := os.Stat(report.SafetyCopy)
	if err != nil {
		t.Fatalf("copie absente : %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions de la copie = %o, attendu 600", perm)
	}

	// La base servie elle-même a bien été migrée et garde la note.
	if got := countNotes(t, upgraded); got != 1 {
		t.Errorf("notes après upgrade = %d, attendu 1", got)
	}
	var tables int
	if err := upgraded.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'attachments'`,
	).Scan(&tables); err != nil || tables != 1 {
		t.Errorf("table attachments absente après upgrade (%d, %v)", tables, err)
	}
}

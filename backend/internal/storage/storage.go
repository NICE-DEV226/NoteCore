// Package storage ouvre la base SQLite et applique les migrations.
// Outre le cahier des charges (§5.3, §7) et l'ADR-002, SQLite
// est l'unique source de vérité : contenu Markdown et métadonnées dans
// la même base, dans des transactions courtes.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // driver SQLite en Go pur : pas de cgo (ADR-005)
)

// Connexion unique et bornée : un écrivain littéral, la concurrence
// applicative sérialisée par Go. Le pool ne s'élargit pas à l'occasion.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("chemin de base vide")
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("répertoire de la base : %w", err)
		}
	}

	// PRAGMA posés à l'ouverture, pas de valeurs implicites.
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("ouverture : %w", err)
	}
	// Pool étroit (1 connexion) : la BD est invoquée par des transactions
	// courtes ; la durée des opérations est bornée, pas le parallelisme.
	db.SetMaxOpenConns(1)

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// migration est une version numérotée du schéma. Toute migration
// appliquée une fois ne le sera pas deux fois (idempotence testée).
type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{version: 1, sql: `
CREATE TABLE IF NOT EXISTS folders (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL COLLATE NOCASE,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS notes (
	id         TEXT PRIMARY KEY,
	title      TEXT NOT NULL DEFAULT '',
	content    TEXT NOT NULL DEFAULT '',
	folder_id  TEXT REFERENCES folders(id) ON DELETE SET NULL,
	is_pinned  INTEGER NOT NULL DEFAULT 0 CHECK (is_pinned IN (0, 1)),
	tags_text  TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_notes_updated_at ON notes(updated_at);
CREATE INDEX IF NOT EXISTS idx_notes_folder_id  ON notes(folder_id);
CREATE INDEX IF NOT EXISTS idx_notes_is_pinned  ON notes(is_pinned);

CREATE TABLE IF NOT EXISTS tags (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL UNIQUE COLLATE NOCASE
);

CREATE TABLE IF NOT EXISTS note_tags (
	note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
	tag_id  TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
	PRIMARY KEY (note_id, tag_id)
);

CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

-- Recherche FTS5 : table autonome, tenue à jour par le service dans la
-- MÊME transaction que la note (ADR-003). note_id est unindexé : c'est
-- la clé de jointure, pas un terme de recherche.
CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
	title,
	content,
	tags_text,
	note_id UNINDEXED,
	tokenize='unicode61 remove_diacritics 1'
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_folders_name ON folders(name COLLATE NOCASE);
`},
	{version: 2, sql: `
-- Pièces jointes locales (ADR-006) : les octets vivent dans la base,
-- une seule copie, sauvegardée et restaurée avec le reste. Une pièce
-- jointe appartient à une seule note et la suit dans sa suppression.
CREATE TABLE IF NOT EXISTS attachments (
	id         TEXT PRIMARY KEY,
	note_id    TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
	filename   TEXT NOT NULL,
	mime       TEXT NOT NULL,
	size       INTEGER NOT NULL DEFAULT 0,
	data       BLOB NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_attachments_note_id ON attachments(note_id);
`},
}

// SchemaVersion renvoie la version de schéma courante — information
// du manifeste de backup (ADR-004 : le backup est un contrat versionné).
func SchemaVersion(db *sql.DB) (int, error) {
	if _, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
	); err != nil {
		return 0, fmt.Errorf("registre des migrations : %w", err)
	}
	var v int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, fmt.Errorf("lecture de version : %w", err)
	}
	return v, nil
}

// Migrate applique toutes les migrations manquantes, en ordre
// croissant, chacune dans sa propre transaction : si une étape
// échoue, rien n'est partiellement appliqué ; l'état courant reste
// sain et reprenable (durabilité avant tout).
func Migrate(db *sql.DB) error {
	// Amorce idempotente : le registre de versions existe toujours avant
	// d'être interrogé. La version est posée dans la même transaction que
	// le schéma correspondant : crash au milieu = transaction annulée,
	// schéma inchangé.
	if _, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
	); err != nil {
		return fmt.Errorf("registre des migrations : %w", err)
	}

	var current int
	if err := db.QueryRow(
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`,
	).Scan(&current); err != nil {
		return fmt.Errorf("lecture des versions : %w", err)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("migration %d : début : %w", m.version, err)
		}
		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d : %w", m.version, err)
		}
		// La version est posée dans la MÊME transaction que le schéma :
		// crash au milieu = transaction annulée, schéma inchangé.
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
			m.version, time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d : version : %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migration %d : commit : %w", m.version, err)
		}
	}
	return nil
}

// Package storage ouvre la base SQLite et applique les migrations.
// Outre le cahier des charges (§5.3, §7) et l'ADR-002, SQLite
// est l'unique source de vérité : contenu Markdown et métadonnées dans
// la même base, dans des transactions courtes.
package storage

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // driver SQLite en Go pur : pas de cgo (ADR-005)
)

// Open ouvre la base et applique les migrations (chemin court, sans
// rapport — les tests et les outils l'utilisent ; l'application passe
// par OpenWithReport).
func Open(path string) (*sql.DB, error) {
	db, _, err := OpenWithReport(path)
	return db, err
}

// MigrationReport décrit ce qui s'est passé à l'ouverture d'une base
// existante. Une installation plus récente doit conserver les données de
// la précédente : lorsqu'une migration est sur le point de modifier le
// schéma, une copie de sécurité de la base est écrite à côté d'elle et
// son chemin est annoncé (§11, critère 12 — rien n'est perdu à
// l'upgrade). `SafetyCopy` est vide quand aucune migration n'était
// requise (installation neuve, ou déjà à jour).
type MigrationReport struct {
	// Chemin de la copie écrite avant migration, ou "" si rien n'a bougé.
	SafetyCopy string
	// Versions de migration effectivement appliquées à cette ouverture.
	Applied []int
}

// OpenWithReport ouvre la base, applique les migrations et renvoie ce
// qu'il s'est passé. C'est l'entrée utilisée par l'application.
func OpenWithReport(path string) (*sql.DB, MigrationReport, error) {
	if path == "" {
		return nil, MigrationReport{}, fmt.Errorf("chemin de base vide")
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, MigrationReport{}, fmt.Errorf("répertoire de la base : %w", err)
		}
	}

	// Une installation neuve n'a rien à conserver : le fichier n'existait
	// pas encore. Seule une base déjà présente chez l'utilisateur — donc
	// celle de la version précédente — mérite une copie avant migration.
	preexisting := true
	if _, err := os.Stat(path); err != nil {
		preexisting = false
	}

	// PRAGMA posés à l'ouverture, pas de valeurs implicites. Connexion
	// unique et bornée : le pool ne s'élargit pas à l'occasion.
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, MigrationReport{}, fmt.Errorf("ouverture : %w", err)
	}
	db.SetMaxOpenConns(1)

	report, err := migrate(db, path, preexisting)
	if err != nil {
		db.Close()
		return nil, report, err
	}
	return db, report, nil
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
	{version: 3, sql: `
-- Révision espacée (ADR-007). Les cartes sont DÉRIVÉES du Markdown des notes
-- (« {{c1::trou}} », « Question :: Réponse ») : la note reste l'unique source
-- du texte, ces tables ne portent que l'état d'apprentissage. Une carte suit
-- sa note dans la suppression (ON DELETE CASCADE).
CREATE TABLE IF NOT EXISTS cards (
	id              TEXT PRIMARY KEY,
	note_id         TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
	key             TEXT NOT NULL,
	kind            TEXT NOT NULL CHECK (kind IN ('cloze', 'qa')),
	front           TEXT NOT NULL,
	back            TEXT NOT NULL,
	due             TEXT NOT NULL,
	stability       REAL NOT NULL DEFAULT 0,
	difficulty      REAL NOT NULL DEFAULT 0,
	scheduled_days  INTEGER NOT NULL DEFAULT 0,
	reps            INTEGER NOT NULL DEFAULT 0,
	lapses          INTEGER NOT NULL DEFAULT 0,
	state           INTEGER NOT NULL DEFAULT 0 CHECK (state BETWEEN 0 AND 3),
	last_review     TEXT NOT NULL DEFAULT '',
	remaining_steps INTEGER NOT NULL DEFAULT 0,
	orphaned_at     TEXT,
	created_at      TEXT NOT NULL,
	UNIQUE (note_id, key)
);
CREATE INDEX IF NOT EXISTS idx_cards_due ON cards(state, due) WHERE orphaned_at IS NULL;

-- Historique de chaque réponse. « before » conserve la carte telle qu'elle
-- était, pour annuler la dernière réponse sans approximation.
CREATE TABLE IF NOT EXISTS review_log (
	id          TEXT PRIMARY KEY,
	card_id     TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
	rating      INTEGER NOT NULL CHECK (rating BETWEEN 1 AND 4),
	reviewed_at TEXT NOT NULL,
	state       INTEGER NOT NULL,
	before      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_review_log_card ON review_log(card_id);
CREATE INDEX IF NOT EXISTS idx_review_log_at ON review_log(reviewed_at);
`},
	{version: 4, sql: `
-- Dossiers imbriqués (ADR-009). Supprimer un dossier ne détruit jamais ses
-- sous-dossiers : l'application les remonte d'un niveau ; ON DELETE SET NULL
-- n'est que le filet de sécurité de la base.
ALTER TABLE folders ADD COLUMN parent_id TEXT REFERENCES folders(id) ON DELETE SET NULL;
-- Le nom est désormais unique PARMI LES DOSSIERS DE MÊME PARENT : deux matières
-- peuvent chacune avoir un dossier « TD ».
DROP INDEX IF EXISTS idx_folders_name;
CREATE UNIQUE INDEX IF NOT EXISTS idx_folders_parent_name ON folders(COALESCE(parent_id, ''), name COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS idx_folders_parent ON folders(parent_id);
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

// Migrate applique toutes les migrations manquantes, sans copie de
// sécurité (chemin court, utilisé par les tests et par les outils qui
// n'ont pas de fichier sous la main). L'application passe par
// OpenWithReport, qui conserve la base d'avant migration.
func Migrate(db *sql.DB) error {
	_, err := migrate(db, "", false)
	return err
}

// migrate applique toutes les migrations manquantes, en ordre
// croissant, chacune dans sa propre transaction : si une étape échoue,
// rien n'est partiellement appliqué ; l'état courant reste sain et
// reprenable (durabilité avant tout).
//
// Quand une migration est nécessaire et que la base était déjà présente
// chez l'utilisateur (`preexisting` — une installation neuve n'a rien à
// conserver), une copie est écrite AVANT la moindre écriture : une
// version plus récente de NoteCore ouvre donc toujours les données de la
// précédente sans les détruire, et un échec de migration reste
// rattrapable (§11, critère 12).
func migrate(db *sql.DB, path string, preexisting bool) (MigrationReport, error) {
	report := MigrationReport{}

	// Amorce idempotente : le registre de versions existe toujours avant
	// d'être interrogé. La version est posée dans la même transaction que
	// le schéma correspondant : crash au milieu = transaction annulée,
	// schéma inchangé.
	if _, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
	); err != nil {
		return report, fmt.Errorf("registre des migrations : %w", err)
	}

	var current int
	if err := db.QueryRow(
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`,
	).Scan(&current); err != nil {
		return report, fmt.Errorf("lecture des versions : %w", err)
	}

	pending := 0
	for _, m := range migrations {
		if m.version > current {
			pending++
		}
	}
	if pending == 0 {
		return report, nil // base déjà à jour : rien n'est touché
	}

	if path != "" && preexisting {
		// Une copie impossible n'empêche pas la migration : elle est
		// signalée, jamais silencieuse, et l'ouverture continue.
		if copy, err := preMigrationCopy(db, path); err != nil {
			log.Printf("stockage : copie avant migration impossible : %v", err)
		} else {
			report.SafetyCopy = copy
		}
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return report, fmt.Errorf("migration %d : début : %w", m.version, err)
		}
		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return report, fmt.Errorf("migration %d : %w", m.version, err)
		}
		// La version est posée dans la MÊME transaction que le schéma :
		// crash au milieu = transaction annulée, schéma inchangé.
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
			m.version, time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			tx.Rollback()
			return report, fmt.Errorf("migration %d : version : %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return report, fmt.Errorf("migration %d : commit : %w", m.version, err)
		}
		report.Applied = append(report.Applied, m.version)
	}
	return report, nil
}

// preMigrationCopy écrit une copie cohérente de la base à côté d'elle
// (même mécanisme que la sauvegarde : VACUUM INTO), en 0600, avec un nom
// daté qui indique d'où elle vient. VACUUM INTO refuse de réécrire un
// fichier existant : on cherche un nom libre plutôt que d'écraser quoi
// que ce soit de l'utilisateur.
func preMigrationCopy(db *sql.DB, path string) (string, error) {
	now := time.Now().UTC()
	for i := 0; i < 10; i++ {
		suffix := now.Format("20060102-150405")
		if i > 0 {
			suffix = fmt.Sprintf("%s-%d", suffix, i)
		}
		dest := fmt.Sprintf("%s.avant-migration-%s", path, suffix)
		if _, err := os.Stat(dest); err == nil {
			continue // nom déjà pris : on tente le suivant
		}
		if _, err := db.Exec(`VACUUM INTO ?`, dest); err != nil {
			return "", fmt.Errorf("copie avant migration : %w", err)
		}
		if err := os.Chmod(dest, 0o600); err != nil {
			return "", fmt.Errorf("permissions de la copie : %w", err)
		}
		return dest, nil
	}
	return "", fmt.Errorf("copie avant migration : aucun nom disponible")
}

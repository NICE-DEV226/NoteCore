// Sauvegarde complète de la base : VACUUM INTO (copie cohérente par le
// moteur, checkpoint WAL inclus), manifeste versionné avec empreinte
// SHA-256, écriture .tmp puis renommage atomique. Validation avant tout
// usage du fichier — jamais d'application silencieuse (ADR-004).
package transfer

import (
	"archive/zip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Bornes de sécurité. Elles s'appliquent AVANT toute écriture : une
// archive hostile (bombe zip, manifeste géant, entrées innombrables)
// est refusée sans toucher au disque ni à la base courante (ADR-004).
const (
	// backupFormatVersion est la version du conteneur de sauvegarde
	// (la disposition de l'archive et du manifeste, pas le schéma).
	backupFormatVersion = 1
	// maxDatabaseBytes borne la base décompressée : au-delà, refus.
	maxDatabaseBytes int64 = 512 << 20 // 512 Mio
	// maxManifestBytes borne la lecture du manifeste en mémoire.
	maxManifestBytes int64 = 1 << 20 // 1 Mio
	// maxArchiveEntries borne le nombre d'entrées de l'archive.
	maxArchiveEntries = 64
)

// tablesBackup liste les tables qu'une sauvegarde exploitable doit
// contenir au minimum. Une base à l'intégrité correcte mais au schéma
// étranger est écartée avant toute restauration.
var tablesBackup = []string{
	"folders", "notes", "tags", "note_tags", "settings", "notes_fts",
}

// schemaAddedTables décrit les tables introduites par version de schéma.
// Les exiger seulement à partir de leur version laisse les sauvegardes
// plus anciennes restaurables (ADR-006 : `attachments` en schéma 2).
var schemaAddedTables = map[int][]string{
	2: {"attachments"},
	3: {"cards", "review_log"},
}

// requiredTables renvoie les tables attendues pour un schéma donné.
func requiredTables(schemaVersion int) []string {
	tables := append([]string{}, tablesBackup...)
	for v := 2; v <= schemaVersion; v++ {
		tables = append(tables, schemaAddedTables[v]...)
	}
	return tables
}

// BackupManifest est le manifeste d'une sauvegarde complète.
type BackupManifest struct {
	Format        string `json:"format"`
	Version       int    `json:"version"`
	SchemaVersion int    `json:"schemaVersion"`
	CreatedAt     string `json:"createdAt"`
	Database      string `json:"database"`
	SHA256        string `json:"sha256"`
	Size          int64  `json:"size"`
	// Notes compte les notes contenues dans la sauvegarde. Présenté dans
	// l'aperçu avant restauration pour confirmer la bonne archive ; vaut 0
	// pour les sauvegardes antérieures à ce champ.
	Notes int `json:"notes"`
}

// CheckRestorable vérifie qu'une sauvegarde validée peut être appliquée
// par l'application courante : une sauvegarde issue d'un schéma PLUS
// RÉCENT est refusée (la restaurer risquerait une perte silencieuse de
// colonnes). Une version courante <= 0 désactive ce contrôle.
func CheckRestorable(manifest *BackupManifest, currentSchemaVersion int) error {
	if manifest == nil {
		return fmt.Errorf("sauvegarde non validée")
	}
	if currentSchemaVersion > 0 && manifest.SchemaVersion > currentSchemaVersion {
		return fmt.Errorf(
			"sauvegarde issue d'une version plus récente (schéma %d > %d) : mettez NoteCore à jour avant de restaurer",
			manifest.SchemaVersion, currentSchemaVersion,
		)
	}
	return nil
}

// RestoreBackup valide puis restaure le contenu d'une sauvegarde complète.
// La base courante reste inchangée tant que l'archive n'est pas validée.
//
// currentSchemaVersion est la version de schéma de l'application : une
// sauvegarde issue d'un schéma PLUS RÉCENT est refusée (la restaurer
// risquerait une perte silencieuse de colonnes). Une valeur <= 0
// désactive ce contrôle.
func RestoreBackup(db *sql.DB, currentSchemaVersion int, path string) error {
	manifest, err := ValidateBackup(path)
	if err != nil {
		return err
	}
	if err := CheckRestorable(manifest, currentSchemaVersion); err != nil {
		return err
	}

	// La base extraite est bornée par la taille annoncée au manifeste :
	// ce que la validation a vérifié est exactement ce qui est restauré.
	tmpDB, err := extractDatabase(path, manifest.Size)
	if err != nil {
		return err
	}
	defer os.Remove(tmpDB)

	if err := checkDatabaseFile(tmpDB, manifest.SchemaVersion); err != nil {
		return err
	}
	if _, err := db.Exec(`ATTACH DATABASE ? AS restore_source`, tmpDB); err != nil {
		return fmt.Errorf("préparation de la restauration : %w", err)
	}
	defer db.Exec(`DETACH DATABASE restore_source`)

	// Une sauvegarde de schéma 1 n'a pas de table `attachments` : on ne
	// copie cette table que si la source la contient réellement.
	hasAttachments, err := attachedTableExists(db, "restore_source", "attachments")
	if err != nil {
		return err
	}
	// Même règle pour les cartes de révision (schéma 3, ADR-007).
	hasCards, err := attachedTableExists(db, "restore_source", "cards")
	if err != nil {
		return err
	}
	hasReviewLog, err := attachedTableExists(db, "restore_source", "review_log")
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("début de la restauration : %w", err)
	}
	rollback := func(cause error) error {
		_ = tx.Rollback()
		return cause
	}

	// Colonnes explicites : la restauration reste correcte même si un
	// futur schéma ajoute des colonnes positionnées autrement.
	// L'historique et les cartes passent avant les notes : ils en dépendent.
	statements := []string{
		`DELETE FROM review_log`,
		`DELETE FROM cards`,
		`DELETE FROM note_tags`,
		`DELETE FROM tags`,
		`DELETE FROM notes_fts`,
	}
	if hasAttachments {
		statements = append(statements, `DELETE FROM attachments`)
	}
	statements = append(statements,
		`DELETE FROM notes`,
		`DELETE FROM folders`,
		`DELETE FROM settings`,
		`INSERT INTO folders(id, name, created_at, updated_at)
			SELECT id, name, created_at, updated_at FROM restore_source.folders`,
		`INSERT INTO notes(id, title, content, folder_id, is_pinned, tags_text, created_at, updated_at)
			SELECT id, title, content, folder_id, is_pinned, tags_text, created_at, updated_at FROM restore_source.notes`,
	)
	if hasAttachments {
		statements = append(statements,
			`INSERT INTO attachments(id, note_id, filename, mime, size, data, created_at)
				SELECT id, note_id, filename, mime, size, data, created_at FROM restore_source.attachments`,
		)
	}
	if hasCards {
		statements = append(statements,
			`INSERT INTO cards(id, note_id, key, kind, front, back, due, stability, difficulty,
			                   scheduled_days, reps, lapses, state, last_review, remaining_steps, orphaned_at, created_at)
				SELECT id, note_id, key, kind, front, back, due, stability, difficulty,
				       scheduled_days, reps, lapses, state, last_review, remaining_steps, orphaned_at, created_at
				  FROM restore_source.cards`,
		)
		if hasReviewLog {
			statements = append(statements,
				`INSERT INTO review_log(id, card_id, rating, reviewed_at, state, before)
					SELECT id, card_id, rating, reviewed_at, state, before FROM restore_source.review_log`,
			)
		}
	}
	statements = append(statements,
		`INSERT INTO tags(id, name) SELECT id, name FROM restore_source.tags`,
		`INSERT INTO note_tags(note_id, tag_id) SELECT note_id, tag_id FROM restore_source.note_tags`,
		`INSERT INTO settings(key, value) SELECT key, value FROM restore_source.settings`,
		`INSERT INTO notes_fts(title, content, tags_text, note_id)
			SELECT title, content, tags_text, id FROM notes`,
	)
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return rollback(fmt.Errorf("restauration : %w", err))
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("validation finale de la restauration : %w", err)
	}
	return nil
}

// extractDatabase extrait la base de l'archive dans un fichier temporaire
// en 0600. La lecture est bornée par maxDatabaseBytes et la taille écrite
// doit correspondre à expectedSize (> 0) : on ne fait pas confiance au
// seul manifeste, on vérifie l'octet écrit.
func extractDatabase(path string, expectedSize int64) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("ouverture de la sauvegarde : %w", err)
	}
	defer zr.Close()
	for _, entry := range zr.File {
		if entry.Name != "notecore.db" {
			continue
		}
		if entry.UncompressedSize64 > uint64(maxDatabaseBytes) {
			return "", fmt.Errorf("base sauvegardée trop volumineuse (%d octets)", entry.UncompressedSize64)
		}
		tmp, err := os.CreateTemp("", "notecore-restore-*.db")
		if err != nil {
			return "", fmt.Errorf("fichier temporaire de restauration : %w", err)
		}
		reader, err := entry.Open()
		var written int64
		if err == nil {
			written, err = io.Copy(tmp, io.LimitReader(reader, maxDatabaseBytes+1))
			reader.Close()
		}
		switch {
		case err == nil && written > maxDatabaseBytes:
			err = fmt.Errorf("base extraite trop volumineuse (%d octets)", written)
		case err == nil && expectedSize > 0 && written != expectedSize:
			err = fmt.Errorf("taille extraite divergente : %d octets lus, %d annoncés", written, expectedSize)
		}
		closeErr := tmp.Close()
		if err != nil {
			os.Remove(tmp.Name())
			return "", fmt.Errorf("extraction de la base : %w", err)
		}
		if closeErr != nil {
			os.Remove(tmp.Name())
			return "", fmt.Errorf("fermeture de la base temporaire : %w", closeErr)
		}
		return tmp.Name(), nil
	}
	return "", fmt.Errorf("archive incomplète : la base est absente")
}

// checkDatabaseFile ouvre la base restaurée en lecture seule et vérifie
// son intégrité ainsi que la présence des tables attendues pour le
// schéma annoncé par le manifeste.
func checkDatabaseFile(path string, schemaVersion int) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("ouverture de la base sauvegardée : %w", err)
	}
	defer db.Close()
	var result string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		return fmt.Errorf("vérification de la base sauvegardée : %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("base sauvegardée invalide : %s", result)
	}
	for _, table := range requiredTables(schemaVersion) {
		exists, err := tableExists(db, table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("sauvegarde incompatible : table « %s » absente", table)
		}
	}
	return nil
}

// countNotes compte les notes d'une copie de base, pour le manifeste et
// l'aperçu de restauration. La copie existe déjà : lecture en mode seul.
func countNotes(path string) (int, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("ouverture de la copie : %w", err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&count); err != nil {
		return 0, fmt.Errorf("comptage des notes : %w", err)
	}
	return count, nil
}

func tableExists(db *sql.DB, name string) (bool, error) {
	var found int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table', 'view') AND name = ?`, name,
	).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("inspection du schéma sauvegardé : %w", err)
	}
	return found > 0, nil
}

// attachedTableExists teste la présence d'une table dans une base
// attachée (`schema.sqlite_master`).
func attachedTableExists(db *sql.DB, schema, name string) (bool, error) {
	var found int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM `+schema+`.sqlite_master WHERE type IN ('table', 'view') AND name = ?`,
		name,
	).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("inspection du schéma sauvegardé : %w", err)
	}
	return found > 0, nil
}

// CreateBackup produit une sauvegarde zip cohérente à destination dest.
func CreateBackup(db *sql.DB, schemaVersion int, dest string) error {
	tmpDB := dest + ".vacuum.tmp"

	// 1. Copie cohérente par le moteur lui-même.
	if _, err := db.Exec("VACUUM INTO ?", tmpDB); err != nil {
		return fmt.Errorf("copie de la base : %w", err)
	}
	defer os.Remove(tmpDB)
	// La copie contient toutes les notes : elle ne doit être lisible que
	// par l'utilisateur, quel que soit l'umask.
	if err := os.Chmod(tmpDB, 0o600); err != nil {
		return fmt.Errorf("permissions de la copie : %w", err)
	}

	sha, size, err := checksumFile(tmpDB)
	if err != nil {
		return err
	}
	noteCount, err := countNotes(tmpDB)
	if err != nil {
		return err
	}
	manifest := BackupManifest{
		Format:        FormatBackup,
		Version:       backupFormatVersion,
		SchemaVersion: schemaVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Database:      "notecore.db",
		SHA256:        sha,
		Size:          size,
		Notes:         noteCount,
	}
	manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("manifeste : %w", err)
	}

	// 2. ZIP -> .tmp dans le MÊME répertoire, puis renommage atomique.
	tmpZip := dest + ".tmp"
	if err := writeZip(tmpZip, tmpDB, manifestRaw); err != nil {
		os.Remove(tmpZip)
		return err
	}
	if err := os.Rename(tmpZip, dest); err != nil {
		os.Remove(tmpZip)
		return fmt.Errorf("installation de la sauvegarde : %w", err)
	}
	// Le fichier renommé remplace un éventuel prédécesseur aux
	// permissions plus larges.
	if err := os.Chmod(dest, 0o600); err != nil {
		return fmt.Errorf("permissions de la sauvegarde : %w", err)
	}
	// Le renommage est durable : on synchronise le répertoire parent.
	if err := syncDir(filepath.Dir(dest)); err != nil {
		return err
	}
	return nil
}

// writeZip écrit la base d'abord, le manifeste en dernier, en 0600, et
// force l'écriture sur le disque avant de rendre la main.
func writeZip(tmpZip, tmpDB string, manifestRaw []byte) error {
	out, err := os.OpenFile(tmpZip, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("création du fichier temporaire : %w", err)
	}

	zw := zip.NewWriter(out)

	dbEntry, err := zw.Create("notecore.db")
	if err != nil {
		out.Close()
		return fmt.Errorf("entrée base : %w", err)
	}
	src, err := os.Open(tmpDB)
	if err != nil {
		out.Close()
		return fmt.Errorf("lecture de la copie : %w", err)
	}
	if _, err := io.Copy(dbEntry, src); err != nil {
		src.Close()
		out.Close()
		return fmt.Errorf("copie base : %w", err)
	}
	src.Close()

	manifestEntry, err := zw.Create("manifest.json")
	if err != nil {
		out.Close()
		return fmt.Errorf("entrée manifeste : %w", err)
	}
	if _, err := manifestEntry.Write(manifestRaw); err != nil {
		out.Close()
		return fmt.Errorf("écriture manifeste : %w", err)
	}

	if err := zw.Close(); err != nil {
		out.Close()
		return fmt.Errorf("clôture zip : %w", err)
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return fmt.Errorf("synchronisation de la sauvegarde : %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("clôture fichier : %w", err)
	}
	return nil
}

// syncDir force l'écriture des métadonnées du répertoire (durabilité du
// renommage). Best-effort selon les systèmes de fichiers.
func syncDir(dir string) error {
	if dir == "" {
		dir = "."
	}
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("synchronisation du répertoire : %w", err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("synchronisation du répertoire : %w", err)
	}
	return nil
}

// checksumFile calcule l'empreinte et la taille d'un fichier.
func checksumFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("lecture : %w", err)
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("hash : %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// ValidateBackup ouvre une archive et évalue son manifeste + empreinte.
// Toute incohérence (version inconnue, schéma absent, empreinte ou taille
// divergentes) est refusée : le fichier ne sera jamais appliqué.
func ValidateBackup(path string) (*BackupManifest, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("archive illisible : %w", err)
	}
	defer zr.Close()

	if len(zr.File) > maxArchiveEntries {
		return nil, fmt.Errorf("archive suspecte : %d entrées", len(zr.File))
	}

	var dbEntry, manifestEntry *zip.File
	for _, entry := range zr.File {
		switch entry.Name {
		case "notecore.db":
			dbEntry = entry
		case "manifest.json":
			manifestEntry = entry
		}
	}
	if dbEntry == nil {
		return nil, fmt.Errorf("archive incomplète : la base est absente")
	}
	if manifestEntry == nil {
		return nil, fmt.Errorf("archive incomplète : le manifeste est absent")
	}
	if dbEntry.UncompressedSize64 > uint64(maxDatabaseBytes) {
		return nil, fmt.Errorf("sauvegarde trop volumineuse (%d octets)", dbEntry.UncompressedSize64)
	}
	if manifestEntry.UncompressedSize64 > uint64(maxManifestBytes) {
		return nil, fmt.Errorf("manifeste anormalement volumineux")
	}

	manifestRaw, err := readZipEntry(manifestEntry, maxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("manifeste illisible : %w", err)
	}
	var manifest BackupManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, fmt.Errorf("manifeste invalide : %w", err)
	}
	if manifest.Format != FormatBackup {
		return nil, fmt.Errorf("format inconnu %q (attendu %s)", manifest.Format, FormatBackup)
	}
	if manifest.Version <= 0 || manifest.Version > backupFormatVersion {
		return nil, fmt.Errorf("version de sauvegarde non prise en charge : %d", manifest.Version)
	}
	if manifest.SchemaVersion <= 0 {
		return nil, fmt.Errorf("manifeste invalide : version de schéma absente")
	}
	if manifest.SHA256 == "" {
		return nil, fmt.Errorf("manifeste invalide : empreinte absente")
	}
	if manifest.Size <= 0 {
		return nil, fmt.Errorf("manifeste invalide : taille absente")
	}
	if manifest.Size > maxDatabaseBytes {
		return nil, fmt.Errorf("sauvegarde trop volumineuse (%d octets)", manifest.Size)
	}

	sha, size, err := checksumZipEntry(dbEntry, maxDatabaseBytes)
	if err != nil {
		return nil, err
	}
	if size != manifest.Size {
		return nil, fmt.Errorf("taille divergente : %d octets lus, %d annoncés", size, manifest.Size)
	}
	if manifest.SHA256 != sha {
		return nil, fmt.Errorf("empreinte divergente : la sauvegarde est corrompue")
	}
	return &manifest, nil
}

// readZipEntry lit un fichier du zip en mémoire, borné par limit.
func readZipEntry(entry *zip.File, limit int64) ([]byte, error) {
	rc, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("entrée %q trop volumineuse", entry.Name)
	}
	return data, nil
}

// checksumZipEntry empreinte + taille d'un fichier du zip, sans
// décompression complète en mémoire de la base si elle est énorme.
// La lecture est bornée par limit : au-delà, l'archive est refusée.
func checksumZipEntry(entry *zip.File, limit int64) (string, int64, error) {
	rc, err := entry.Open()
	if err != nil {
		return "", 0, fmt.Errorf("lecture base : %w", err)
	}
	defer rc.Close()

	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(rc, limit+1))
	if err != nil {
		return "", 0, fmt.Errorf("hash de la base : %w", err)
	}
	if n > limit {
		return "", 0, fmt.Errorf("base sauvegardée trop volumineuse (%d octets)", n)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

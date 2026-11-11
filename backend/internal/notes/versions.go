package notes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// maxVersionsPerNote borne l'historique : au-delà, les plus anciennes
// versions d'une note sont supprimées. Une session d'écriture intensive
// produit une version par sauvegarde distincte ; 100 couvre largement
// l'usage sans gonfler la base.
const maxVersionsPerNote = 100

// VersionInfo décrit une version sans charger tout son contenu : la
// liste affiche date, titre et taille, le contenu n'est lu qu'à la
// demande (GetVersion).
type VersionInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	Chars     int    `json:"chars"`
}

// Version est une version complète, prête à être restaurée ou prévisualisée.
type Version struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

// recordVersionInTx mémorise l'état courant de la note dans la MÊME
// transaction que sa sauvegarde. Appelée uniquement quand titre ou
// contenu a changé, et sautée si l'état est identique à la dernière
// version — les sauvegardes répétées ne dupliquent rien. La purge des
// versions au-delà du plafond se fait ici aussi : transaction courte.
func recordVersionInTx(ctx context.Context, tx *sql.Tx, noteID, title, content string) error {
	var lastTitle, lastContent string
	err := tx.QueryRowContext(ctx, `
		SELECT title, content FROM note_versions
		 WHERE note_id = ? ORDER BY created_at DESC, rowid DESC LIMIT 1`, noteID).
		Scan(&lastTitle, &lastContent)
	if err == nil && lastTitle == title && lastContent == content {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("lecture dernière version : %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO note_versions(id, note_id, title, content, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		uuid.NewString(), noteID, title, content,
		time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("enregistrement de la version : %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM note_versions
		 WHERE note_id = ? AND id NOT IN (
			SELECT id FROM note_versions WHERE note_id = ?
			 ORDER BY created_at DESC, rowid DESC LIMIT ?)`,
		noteID, noteID, maxVersionsPerNote); err != nil {
		return fmt.Errorf("purge des anciennes versions : %w", err)
	}
	return nil
}

// ListVersions renvoie l'historique d'une note, le plus récent d'abord.
func (s *Service) ListVersions(ctx context.Context, noteID string) ([]VersionInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, created_at, LENGTH(content)
		  FROM note_versions WHERE note_id = ?
		 ORDER BY created_at DESC, rowid DESC LIMIT ?`, noteID, maxVersionsPerNote)
	if err != nil {
		return nil, fmt.Errorf("lecture des versions : %w", err)
	}
	defer rows.Close()
	var out []VersionInfo
	for rows.Next() {
		var v VersionInfo
		if err := rows.Scan(&v.ID, &v.Title, &v.CreatedAt, &v.Chars); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetVersion renvoie le contenu complet d'une version.
func (s *Service) GetVersion(ctx context.Context, versionID string) (Version, error) {
	var v Version
	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, content, created_at FROM note_versions WHERE id = ?`, versionID).
		Scan(&v.ID, &v.Title, &v.Content, &v.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Version{}, fmt.Errorf("version %q introuvable", versionID)
		}
		return Version{}, fmt.Errorf("lecture de la version : %w", err)
	}
	return v, nil
}

// RestoreVersion rétablit une version : elle passe par UpdateNote, donc
// l'état remplacé devient lui-même une version — la restauration est
// toujours réversible, jamais destructrice.
func (s *Service) RestoreVersion(ctx context.Context, noteID, versionID string) (Note, error) {
	v, err := s.GetVersion(ctx, versionID)
	if err != nil {
		return Note{}, err
	}
	// La version doit appartenir à la note demandée : jamais de copie
	// croisée entre notes.
	var owner string
	if err := s.db.QueryRowContext(ctx,
		`SELECT note_id FROM note_versions WHERE id = ?`, versionID).Scan(&owner); err != nil {
		return Note{}, fmt.Errorf("version %q introuvable", versionID)
	}
	if owner != noteID {
		return Note{}, fmt.Errorf("cette version n'appartient pas à la note")
	}
	title, content := v.Title, v.Content
	return s.UpdateNote(ctx, noteID, Patch{Title: &title, Content: &content})
}

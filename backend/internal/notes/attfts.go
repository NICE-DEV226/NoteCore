package notes

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Index FTS des pièces jointes texte (schéma 7) : le contenu des
// fichiers texte devient cherchable comme celui des notes. Les
// binaires (images, PDF, archives) n'y entrent jamais — on ne devine
// pas leur texte.

// maxIndexedAttachmentBytes borne le texte indexé par pièce jointe :
// au-delà, le fichier reste attaché et visible, simplement non cherchable.
const maxIndexedAttachmentBytes = 512 << 10 // 512 Kio

// maxIndexedAttachmentChars borne le texte stocké dans l'index.
const maxIndexedAttachmentChars = 200_000

// attachmentTextExtensions : extensions indexées même quand le type MIME
// est générique (application/octet-stream) ou absent.
var attachmentTextExtensions = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".csv": true,
	".json": true, ".log": true, ".xml": true, ".yaml": true, ".yml": true,
}

// indexableAttachment dit si le contenu peut entrer dans l'index FTS.
func indexableAttachment(mime, filename string, size int64) bool {
	if size <= 0 || size > maxIndexedAttachmentBytes {
		return false
	}
	if strings.HasPrefix(strings.ToLower(mime), "text/") {
		return true
	}
	for ext := range attachmentTextExtensions {
		if strings.HasSuffix(strings.ToLower(filename), ext) {
			return true
		}
	}
	return false
}

// attachmentText extrait un texte indexable : UTF-8 tolérant, sans octet
// NUL (certaines bases de données et FTS5 n'aiment pas), borné.
func attachmentText(data []byte) string {
	clean := strings.Map(func(r rune) rune {
		if r == 0 || !utf8.ValidRune(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(string(data), "\uFFFD"))
	if len(clean) > maxIndexedAttachmentChars {
		clean = clean[:maxIndexedAttachmentChars]
	}
	return clean
}

// indexAttachmentInTx écrit la pièce jointe dans l'index FTS, dans la
// transaction de son enregistrement : index et donnée restent cohérents.
func indexAttachmentInTx(ctx context.Context, tx *sql.Tx, a Attachment, data []byte) error {
	if !indexableAttachment(a.Mime, a.Filename, a.Size) {
		return nil
	}
	text := attachmentText(data)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO attachments_fts(filename, content, note_id, attachment_id)
		VALUES (?, ?, ?, ?)`, a.Filename, text, a.NoteID, a.ID)
	return err
}

// unindexAttachmentInTx retire une pièce jointe de l'index.
func unindexAttachmentInTx(ctx context.Context, tx *sql.Tx, attachmentID string) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM attachments_fts WHERE attachment_id = ?`, attachmentID)
	return err
}

// unindexNoteAttachmentsInTx retire de l'index toutes les pièces jointes
// des notes matchant la clause WHERE fournie (ex. corbeille, purge).
func unindexNoteAttachmentsInTx(ctx context.Context, tx *sql.Tx, where string, args ...any) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM attachments_fts WHERE note_id IN (`+where+`)`, args...)
	return err
}

// IndexAttachmentTexts indexe les pièces jointes texte existantes qui
// ne le sont pas encore : appelé une fois au démarrage (migration v7),
// idempotent — les pièces déjà indexées sont ignorées.
func (s *Service) IndexAttachmentTexts(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.note_id, a.filename, a.mime, a.size, a.data
		  FROM attachments a
		 WHERE NOT EXISTS (
			SELECT 1 FROM attachments_fts f WHERE f.attachment_id = a.id)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type pending struct {
		a    Attachment
		data []byte
	}
	var batch []pending
	for rows.Next() {
		var a Attachment
		var data []byte
		if err := rows.Scan(&a.ID, &a.NoteID, &a.Filename, &a.Mime, &a.Size, &data); err != nil {
			return err
		}
		if indexableAttachment(a.Mime, a.Filename, a.Size) {
			batch = append(batch, pending{a, data})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range batch {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := indexAttachmentInTx(ctx, tx, p.a, p.data); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// searchAttachmentText cherche dans l'index des pièces jointes et rend
// des résultats rattachés à leur note (NoteID) avec la pièce jointe
// identifiée : l'interface peut ouvrir directement l'aperçu.
func (s *Service) searchAttachmentText(ctx context.Context, match string) ([]SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT n.id, n.title,
		       a.filename,
		       snippet(attachments_fts, 1, '«', '»', '…', 12),
		       n.updated_at,
		       a.id
		  FROM attachments_fts
		  JOIN attachments a ON a.id = attachments_fts.attachment_id
		  JOIN notes n ON n.id = a.note_id
		 WHERE attachments_fts MATCH ? AND n.deleted_at IS NULL
		 ORDER BY bm25(attachments_fts)
		 LIMIT 20`,
		match)
	if err != nil {
		return nil, fmt.Errorf("recherche pièces jointes : %w", err)
	}
	defer rows.Close()

	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.NoteID, &r.Title, &r.AttachmentName, &r.Excerpt, &r.UpdatedAt, &r.AttachmentID); err != nil {
			return nil, fmt.Errorf("scan recherche pièces jointes : %w", err)
		}
		r.Kind = "attachment"
		out = append(out, r)
	}
	return out, rows.Err()
}

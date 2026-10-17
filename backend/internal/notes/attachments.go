// Pièces jointes locales (ADR-006) : images et fichiers rattachés à une
// note. Les octets vivent dans SQLite — une seule copie, sauvegardée et
// restaurée avec le reste (cahier des charges §5.3, §6.2). Une pièce jointe
// appartient à une seule note et disparaît avec elle (ON DELETE CASCADE).
package notes

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// MaxAttachmentBytes borne une pièce jointe unique. Au-delà, l'ajout est
// refusé avec un message lisible plutôt que de gonfler la base sans
// limite (ADR-006).
const MaxAttachmentBytes = 10 << 20 // 10 Mio

// maxFilenameLen borne le nom conservé pour l'export.
const maxFilenameLen = 120

// Attachment décrit une pièce jointe sans ses octets (liste, export).
type Attachment struct {
	ID        string `json:"id"`
	NoteID    string `json:"noteId"`
	Filename  string `json:"filename"`
	Mime      string `json:"mime"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"createdAt"`
}

// AttachmentUpload est la charge reçue de l'interface : nom, type
// déclaré et contenu encodé en base64 (le pont Wails transporte du
// texte, pas des octets bruts).
type AttachmentUpload struct {
	Filename   string `json:"filename"`
	Mime       string `json:"mime"`
	DataBase64 string `json:"dataBase64"`
}

// AttachmentData renvoie les octets pour l'affichage ou l'enregistrement.
type AttachmentData struct {
	Mime       string `json:"mime"`
	Filename   string `json:"filename"`
	DataBase64 string `json:"dataBase64"`
}

// AddAttachment décode, borne et enregistre une pièce jointe rattachée à
// une note existante.
func (s *Service) AddAttachment(ctx context.Context, noteID string, up AttachmentUpload) (Attachment, error) {
	if strings.TrimSpace(noteID) == "" {
		return Attachment{}, fmt.Errorf("note cible non identifiée")
	}
	// Borne AVANT décodage complet : on refuse tôt un contenu trop gros.
	// Une chaîne base64 fait ~4/3 de la taille décodée.
	if maxEncoded := base64.StdEncoding.EncodedLen(int(MaxAttachmentBytes)) + 4; len(up.DataBase64) > maxEncoded {
		return Attachment{}, errAttachmentTooLarge()
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(up.DataBase64))
	if err != nil {
		return Attachment{}, fmt.Errorf("contenu de pièce jointe illisible : %w", err)
	}
	if len(data) == 0 {
		return Attachment{}, fmt.Errorf("pièce jointe vide")
	}
	if int64(len(data)) > MaxAttachmentBytes {
		return Attachment{}, errAttachmentTooLarge()
	}

	name := safeAttachmentName(up.Filename)
	mime := normalizeMime(up.Mime)
	now := time.Now().UTC().Format(time.RFC3339)
	a := Attachment{
		ID:        newID(),
		NoteID:    noteID,
		Filename:  name,
		Mime:      mime,
		Size:      int64(len(data)),
		CreatedAt: now,
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO attachments (id, note_id, filename, mime, size, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.NoteID, a.Filename, a.Mime, a.Size, data, a.CreatedAt); err != nil {
		if isForeignKeyErr(err) {
			return Attachment{}, fmt.Errorf("note %q introuvable", noteID)
		}
		return Attachment{}, fmt.Errorf("enregistrement pièce jointe : %w", err)
	}
	return a, nil
}

// ListAttachments renvoie les métadonnées des pièces jointes d'une note,
// sans les octets (utilisé par l'export).
func (s *Service) ListAttachments(ctx context.Context, noteID string) ([]Attachment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, note_id, filename, mime, size, created_at
		  FROM attachments WHERE note_id = ? ORDER BY created_at`, noteID)
	if err != nil {
		return nil, fmt.Errorf("lecture pièces jointes : %w", err)
	}
	defer rows.Close()

	list := make([]Attachment, 0)
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.NoteID, &a.Filename, &a.Mime, &a.Size, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("lecture pièce jointe : %w", err)
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// GetAttachmentData lit les octets d'une pièce jointe (aperçu, export).
func (s *Service) GetAttachmentData(ctx context.Context, id string) (AttachmentData, error) {
	var (
		a    Attachment
		data []byte
	)
	row := s.db.QueryRowContext(ctx, `
		SELECT id, note_id, filename, mime, size, created_at, data
		  FROM attachments WHERE id = ?`, id)
	if err := row.Scan(&a.ID, &a.NoteID, &a.Filename, &a.Mime, &a.Size, &a.CreatedAt, &data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AttachmentData{}, fmt.Errorf("pièce jointe %q introuvable", id)
		}
		return AttachmentData{}, fmt.Errorf("lecture pièce jointe : %w", err)
	}
	return AttachmentData{
		Mime:       a.Mime,
		Filename:   a.Filename,
		DataBase64: base64.StdEncoding.EncodeToString(data),
	}, nil
}

// DeleteAttachment retire une pièce jointe précise.
func (s *Service) DeleteAttachment(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM attachments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("suppression pièce jointe : %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return fmt.Errorf("pièce jointe %q introuvable", id)
	}
	return nil
}

// --- helpers ------------------------------------------------------------

func errAttachmentTooLarge() error {
	return fmt.Errorf("pièce jointe trop volumineuse (maximum %d Mio)", MaxAttachmentBytes>>20)
}

// safeAttachmentName ne conserve que le nom de base, retire les
// caractères de contrôle et bornes la longueur. Le nom ne sert qu'à
// l'affichage et à l'export : jamais à construire un chemin.
func safeAttachmentName(raw string) string {
	name := filepath.Base(strings.TrimSpace(raw))
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	name = strings.Trim(name, ".")
	if name == "" {
		return "fichier"
	}
	if len(name) > maxFilenameLen {
		// Conserve l'extension en coupant le radical, pas la fin.
		ext := filepath.Ext(name)
		if len(ext) > 16 {
			ext = ""
		}
		keep := maxFilenameLen - len(ext)
		if keep < 1 {
			keep = 1
		}
		name = name[:keep] + ext
	}
	return name
}

// normalizeMime borne et normalise le type déclaré. Il reste indicatif :
// l'aperçu ne rend une image que si le préfixe est bien `image/`.
func normalizeMime(raw string) string {
	m := strings.ToLower(strings.TrimSpace(raw))
	if m == "" {
		return "application/octet-stream"
	}
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	if len(m) > 100 {
		return "application/octet-stream"
	}
	return m
}

func isForeignKeyErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToUpper(err.Error()), "FOREIGN KEY")
}

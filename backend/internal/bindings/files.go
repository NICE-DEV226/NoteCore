// Parcourir, ouvrir et lire les pièces jointes (ADR-006, ADR-008). Les octets
// restent dans la base : « ouvrir avec le système » en écrit une copie
// temporaire, supprimée à la fermeture de l'application.
package bindings

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"notecore/backend/internal/notes"
)

// ListAllAttachments renvoie toutes les pièces jointes, avec leur note.
func (n *NoteCore) ListAllAttachments() ([]notes.AttachmentRef, error) {
	return n.svc.ListAllAttachments(n.callContext())
}

// ArchiveListing est le contenu d'une archive .zip jointe.
type ArchiveListing struct {
	Entries   []notes.ArchiveEntry `json:"entries"`
	Truncated bool                 `json:"truncated"`
}

// ListArchive liste une archive .zip jointe, sans rien extraire.
func (n *NoteCore) ListArchive(id string) (ArchiveListing, error) {
	if id == "" {
		return ArchiveListing{}, fmt.Errorf("pièce jointe non identifiée")
	}
	entries, truncated, err := n.svc.ListArchive(n.callContext(), id)
	return ArchiveListing{Entries: entries, Truncated: truncated}, err
}

const tempPrefix = "notecore-open-"

// OpenAttachment ouvre une pièce jointe avec l'application par défaut du
// système. Les types que le système exécuterait (.exe, .bat, .ps1, .sh…) sont
// refusés : une pièce jointe est une donnée, jamais un programme.
func (n *NoteCore) OpenAttachment(id string) error {
	if id == "" {
		return fmt.Errorf("pièce jointe non identifiée")
	}
	ctx := n.callContext()
	data, err := n.svc.GetAttachmentData(ctx, id)
	if err != nil {
		return err
	}
	if notes.IsRiskyFilename(data.Filename) {
		return fmt.Errorf("pour votre sécurité, NoteCore n'ouvre pas « %s » avec une autre application (ce type de fichier peut s'exécuter). Utilisez « Enregistrer sous… » si vous en avez besoin", data.Filename)
	}
	raw, err := base64.StdEncoding.DecodeString(data.DataBase64)
	if err != nil {
		return fmt.Errorf("pièce jointe illisible : %w", err)
	}

	dir, err := os.MkdirTemp("", tempPrefix+"*")
	if err != nil {
		return fmt.Errorf("dossier temporaire : %w", err)
	}
	path := filepath.Join(dir, filepath.Base(data.Filename))
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("copie temporaire : %w", err)
	}
	n.mu.Lock()
	n.tempDirs = append(n.tempDirs, dir)
	n.mu.Unlock()

	wailsruntime.BrowserOpenURL(ctx, fileURL(path))
	return nil
}

// fileURL transforme un chemin local en URL file:// valable sur chaque système.
func fileURL(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") { // Windows : C:/Users/…
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// SweepTemp supprime les copies temporaires laissées par une session qui n'a
// pas pu se fermer proprement (plantage, coupure).
func SweepTemp() {
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), tempPrefix+"*"))
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(m)
		}
	}
}

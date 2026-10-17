// Bindings des pièces jointes locales (ADR-006). Le pont Wails
// transporte du texte : les octets passent en base64. La validation de
// taille et de nom est faite par le paquet notes ; ici on ne fait que
// choisir les destinations via les dialogs système.
package bindings

import (
	"encoding/base64"
	"fmt"
	"os"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"notecore/backend/internal/notes"
)

// AddAttachment enregistre une pièce jointe rattachée à une note et
// renvoie sa description. Refuse au-delà de la taille maximale.
func (n *NoteCore) AddAttachment(noteID, filename, mime, dataBase64 string) (notes.Attachment, error) {
	if noteID == "" {
		return notes.Attachment{}, fmt.Errorf("note cible non identifiée")
	}
	return n.svc.AddAttachment(n.callContext(), noteID, notes.AttachmentUpload{
		Filename:   filename,
		Mime:       mime,
		DataBase64: dataBase64,
	})
}

// ListAttachments renvoie les pièces jointes d'une note, sans les octets.
func (n *NoteCore) ListAttachments(noteID string) ([]notes.Attachment, error) {
	if noteID == "" {
		return nil, fmt.Errorf("note non identifiée")
	}
	return n.svc.ListAttachments(n.callContext(), noteID)
}

// ReadAttachment renvoie les octets d'une pièce jointe (aperçu d'image).
func (n *NoteCore) ReadAttachment(id string) (notes.AttachmentData, error) {
	if id == "" {
		return notes.AttachmentData{}, fmt.Errorf("pièce jointe non identifiée")
	}
	return n.svc.GetAttachmentData(n.callContext(), id)
}

// DeleteAttachment retire une pièce jointe.
func (n *NoteCore) DeleteAttachment(id string) error {
	if id == "" {
		return fmt.Errorf("pièce jointe non identifiée")
	}
	return n.svc.DeleteAttachment(n.callContext(), id)
}

// SaveAttachment écrit une pièce jointe sur le disque, destination
// choisie par l'utilisateur (fichiers non-images : ouverture/partage).
func (n *NoteCore) SaveAttachment(id string) (bool, error) {
	if id == "" {
		return false, fmt.Errorf("pièce jointe non identifiée")
	}
	data, err := n.svc.GetAttachmentData(n.callContext(), id)
	if err != nil {
		return false, err
	}
	raw, err := base64.StdEncoding.DecodeString(data.DataBase64)
	if err != nil {
		return false, fmt.Errorf("pièce jointe illisible : %w", err)
	}
	path, err := wailsruntime.SaveFileDialog(n.callContext(), wailsruntime.SaveDialogOptions{
		Title:           "Enregistrer la pièce jointe",
		DefaultFilename: data.Filename,
	})
	if err != nil {
		return false, fmt.Errorf("dialog d'enregistrement : %w", err)
	}
	if path == "" {
		return false, nil // annulé
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return false, fmt.Errorf("écriture de la pièce jointe : %w", err)
	}
	return true, nil
}

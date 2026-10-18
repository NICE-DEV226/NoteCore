// Extensions de transfert de la facade bindings : export Markdown,
// export multiple (archive), sauvegarde et validation de backup.
// Les chemins de destination sont choisis par les dialogs système
// (ARCHITECTURE.md §7) — jamais construits depuis une entrée libre.
package bindings

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"notecore/backend/internal/storage"
	"notecore/backend/internal/transfer"
)

// --- export ----------------------------------------------------------

// ExportNote écrit une note en Markdown autonome, destination choisie
// par le dialog système. Les pièces jointes sont écrites dans un dossier
// frère `<nom>-assets/` et les références `attachment:<id>` réécrites en
// chemins relatifs, pour que le Markdown reste lisible sans NoteCore.
// Renvoie false si l'utilisateur annule.
func (n *NoteCore) ExportNote(id string) (bool, error) {
	if id == "" {
		return false, fmt.Errorf("note à exporter non identifiée")
	}
	note, err := n.svc.GetNote(n.callContext(), id)
	if err != nil {
		return false, err
	}
	assets, err := n.exportAssetsFor(note.ID)
	if err != nil {
		return false, err
	}
	writes, refs := transfer.ExportAttachmentPlan(note.Title, assets)
	markdown := transfer.ExportNoteMarkdown(note, refs)

	path, err := wailsruntime.SaveFileDialog(n.callContext(), wailsruntime.SaveDialogOptions{
		Title:           "Exporter la note en Markdown",
		DefaultFilename: transfer.SanitizeFilename(note.Title) + ".md",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Markdown (*.md)", Pattern: "*.md"},
		},
	})
	if err != nil {
		return false, fmt.Errorf("dialog d'enregistrement : %w", err)
	}
	if path == "" {
		return false, nil // annulé : ni écriture, ni erreur
	}
	if err := os.WriteFile(path, []byte(markdown), 0o600); err != nil {
		return false, fmt.Errorf("écriture de l'export : %w", err)
	}
	if err := writeExportAssets(filepath.Dir(path), writes); err != nil {
		return false, err
	}
	return true, nil
}

// ExportAllNotes archive toutes les notes (Markdown + pièces jointes +
// manifeste).
func (n *NoteCore) ExportAllNotes() (bool, error) {
	list, err := n.svc.ListNotes(n.callContext())
	if err != nil {
		return false, err
	}
	if len(list) == 0 {
		return false, fmt.Errorf("aucune note à exporter")
	}

	assetsByNote := make(map[string][]transfer.ExportAsset, len(list))
	for _, note := range list {
		assets, err := n.exportAssetsFor(note.ID)
		if err != nil {
			return false, err
		}
		if len(assets) > 0 {
			assetsByNote[note.ID] = assets
		}
	}

	data, err := transfer.ExportAllArchive(list, assetsByNote)
	if err != nil {
		return false, err
	}

	path, err := wailsruntime.SaveFileDialog(n.callContext(), wailsruntime.SaveDialogOptions{
		Title:           "Exporter toutes les notes",
		DefaultFilename: transfer.SuggestedExportName(),
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Archive d'export (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return false, fmt.Errorf("dialog d'enregistrement : %w", err)
	}
	if path == "" {
		return false, nil // annulé
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return false, fmt.Errorf("écriture de l'archive : %w", err)
	}
	return true, nil
}

// exportAssetsFor rassemble les octets des pièces jointes d'une note.
func (n *NoteCore) exportAssetsFor(noteID string) ([]transfer.ExportAsset, error) {
	list, err := n.svc.ListAttachments(n.callContext(), noteID)
	if err != nil {
		return nil, err
	}
	assets := make([]transfer.ExportAsset, 0, len(list))
	for _, a := range list {
		data, err := n.svc.GetAttachmentData(n.callContext(), a.ID)
		if err != nil {
			return nil, err
		}
		raw, err := base64.StdEncoding.DecodeString(data.DataBase64)
		if err != nil {
			return nil, fmt.Errorf("pièce jointe %q illisible : %w", a.Filename, err)
		}
		assets = append(assets, transfer.ExportAsset{ID: a.ID, Filename: a.Filename, Data: raw})
	}
	return assets, nil
}

// writeExportAssets écrit les pièces jointes d'un export de note dans le
// dossier de destination (création atomique du dossier au besoin).
func writeExportAssets(baseDir string, writes []transfer.AttachmentWrite) error {
	for _, w := range writes {
		full := filepath.Join(baseDir, filepath.FromSlash(w.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			return fmt.Errorf("création du dossier de pièces jointes : %w", err)
		}
		if err := os.WriteFile(full, w.Data, 0o600); err != nil {
			return fmt.Errorf("écriture de la pièce jointe %q : %w", w.Path, err)
		}
	}
	return nil
}

// --- sauvegarde -------------------------------------------------------

// CreateBackup produit une sauvegarde complète vérifiable (VACUUM INTO,
// manifeste, empreinte). Renvoie false si l'utilisateur annule.
func (n *NoteCore) CreateBackup() (bool, error) {
	defaultName := fmt.Sprintf("notecore-backup-%s.zip", time.Now().Format("2006-01-02"))
	path, err := wailsruntime.SaveFileDialog(n.callContext(), wailsruntime.SaveDialogOptions{
		Title:           "Sauvegarder les données",
		DefaultFilename: defaultName,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Sauvegarde NoteCore (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return false, fmt.Errorf("dialog d'enregistrement : %w", err)
	}
	if path == "" {
		return false, nil // annulé
	}

	schemaVersion, err := storage.SchemaVersion(n.db)
	if err != nil {
		return false, err
	}
	if err := transfer.CreateBackup(n.db, schemaVersion, path); err != nil {
		return false, err
	}
	return true, nil
}

// RestorePreview décrit la sauvegarde choisie par l'utilisateur, avant
// toute écriture (ADR-004) : la confirmation porte sur un fichier
// identifié, daté et compté, jamais sur une action aveugle.
type RestorePreview struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	CreatedAt     string `json:"createdAt"`
	SchemaVersion int    `json:"schemaVersion"`
	Notes         int    `json:"notes"`
}

// InspectBackup ouvre le sélecteur de fichier et valide la sauvegarde
// choisie SANS rien écrire. Un chemin vide signifie que l'utilisateur a
// annulé. Une archive corrompue, incompatible ou issue d'une version
// plus récente est écartée ici, avant toute confirmation.
func (n *NoteCore) InspectBackup() (RestorePreview, error) {
	path, err := wailsruntime.OpenFileDialog(n.callContext(), wailsruntime.OpenDialogOptions{
		Title: "Restaurer une sauvegarde NoteCore",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Sauvegarde NoteCore (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return RestorePreview{}, fmt.Errorf("dialog de restauration : %w", err)
	}
	if path == "" {
		return RestorePreview{}, nil // annulé
	}

	manifest, err := transfer.ValidateBackup(path)
	if err != nil {
		return RestorePreview{}, err
	}
	schemaVersion, err := storage.SchemaVersion(n.db)
	if err != nil {
		return RestorePreview{}, err
	}
	if err := transfer.CheckRestorable(manifest, schemaVersion); err != nil {
		return RestorePreview{}, err
	}
	return RestorePreview{
		Path:          path,
		Name:          filepath.Base(path),
		CreatedAt:     manifest.CreatedAt,
		SchemaVersion: manifest.SchemaVersion,
		Notes:         manifest.Notes,
	}, nil
}

// RestoreBackup restaure une sauvegarde choisie lors de l'inspection.
// Une copie de sécurité de la base courante est écrite avant tout
// remplacement : une restauration malheureuse reste rattrapable. Son
// chemin est renvoyé pour être annoncé, jamais conservé en silence.
func (n *NoteCore) RestoreBackup(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("sauvegarde non identifiée")
	}
	safety, err := n.safetyCopy()
	if err != nil {
		return "", err
	}
	schemaVersion, err := storage.SchemaVersion(n.db)
	if err != nil {
		_ = os.Remove(safety)
		return "", err
	}
	if err := transfer.RestoreBackup(n.db, schemaVersion, path); err != nil {
		// Rien n'a été appliqué (validation ou transaction annulée) :
		// la copie de sécurité n'a plus d'objet.
		_ = os.Remove(safety)
		return "", err
	}
	return safety, nil
}

// safetyCopy écrit une copie cohérente de la base courante à côté d'elle
// (même mécanisme que la sauvegarde : VACUUM INTO), en 0600.
func (n *NoteCore) safetyCopy() (string, error) {
	var (
		seq  int
		name string
		file string
	)
	if err := n.db.QueryRow(
		`SELECT seq, name, file FROM pragma_database_list WHERE name = 'main'`,
	).Scan(&seq, &name, &file); err != nil {
		return "", fmt.Errorf("localisation de la base : %w", err)
	}
	if file == "" {
		return "", fmt.Errorf("base en mémoire : copie de sécurité impossible")
	}
	dest := fmt.Sprintf("%s.avant-restauration-%s", file, time.Now().UTC().Format("20060102-150405"))
	if _, err := n.db.Exec(`VACUUM INTO ?`, dest); err != nil {
		return "", fmt.Errorf("copie de sécurité : %w", err)
	}
	if err := os.Chmod(dest, 0o600); err != nil {
		return "", fmt.Errorf("permissions de la copie de sécurité : %w", err)
	}
	return dest, nil
}

package share

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"notecore/backend/internal/notes"
	"notecore/backend/internal/transfer"
)

// FormatShare est le contrat du paquet `.notecore`.
const FormatShare = "notecore-share"

// MaxPackageBytes borne un paquet (envoi comme import) : tout est traité en
// mémoire, une borne claire vaut mieux qu'un blocage de l'application.
const MaxPackageBytes = 256 << 20

type Scope string

const (
	ScopeNote   Scope = "note"
	ScopeFolder Scope = "folder"
	ScopeAll    Scope = "all"
)

type Format string

const (
	// FormatNoteCore : paquet `.notecore`, réimportable avec ses pièces jointes.
	FormatNoteCore Format = "notecore"
	// FormatMarkdown : archive .zip de fichiers Markdown, lisible partout.
	FormatMarkdown Format = "markdown"
)

// Selection désigne ce qu'on partage.
type Selection struct {
	Scope Scope
	ID    string // identifiant de la note ou du dossier
}

// Package est un paquet prêt à être enregistré ou envoyé.
type Package struct {
	Data        []byte
	Filename    string
	Notes       int
	Attachments int
	Encrypted   bool
}

// Item décrit une note du paquet.
type Item struct {
	File   string `json:"file"`
	Title  string `json:"title"`
	Folder string `json:"folder,omitempty"` // NOM du dossier : les identifiants ne voyagent pas
	// FolderPath : chemin complet (dossiers imbriqués, ADR-009). Absent des
	// paquets antérieurs, qui n'ont que Folder.
	FolderPath  []string  `json:"folderPath,omitempty"`
	Tags        []string  `json:"tags"`
	Pinned      bool      `json:"pinned,omitempty"`
	CreatedAt   string    `json:"createdAt"`
	UpdatedAt   string    `json:"updatedAt"`
	Attachments []AttItem `json:"attachments,omitempty"`
}

// AttItem décrit une pièce jointe ; son identifiant d'origine sert à réécrire
// les références `attachment:<id>` du texte à l'import.
type AttItem struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Mime     string `json:"mime"`
	File     string `json:"file"`
}

// Manifest est le contenu de manifest.json, écrit en dernier.
type Manifest struct {
	Format    string `json:"format"`
	Version   int    `json:"version"`
	CreatedAt string `json:"createdAt"`
	Count     int    `json:"count"`
	Notes     []Item `json:"notes"`
}

// Build assemble un paquet. Le mot de passe, facultatif, chiffre le paquet
// `.notecore` ; il est refusé pour l'archive Markdown, faite pour être lue
// sans NoteCore.
func Build(ctx context.Context, svc *notes.Service, sel Selection, format Format, passphrase string, now time.Time) (Package, error) {
	if passphrase != "" && format != FormatNoteCore {
		return Package{}, errors.New("le mot de passe n'est disponible que pour le format NoteCore")
	}
	all, err := svc.ListNotes(ctx)
	if err != nil {
		return Package{}, err
	}
	folders, err := svc.ListFolders(ctx)
	if err != nil {
		return Package{}, err
	}
	folderName := make(map[string]string, len(folders))
	for _, f := range folders {
		folderName[f.ID] = f.Name
	}

	var chosen []notes.Note
	base := "notecore-notes-" + now.Format("2006-01-02")
	switch sel.Scope {
	case ScopeNote:
		for _, n := range all {
			if n.ID == sel.ID {
				chosen = append(chosen, n)
				base = transfer.SanitizeFilename(n.Title)
			}
		}
	case ScopeFolder:
		for _, n := range all {
			if n.FolderID != nil && *n.FolderID == sel.ID {
				chosen = append(chosen, n)
			}
		}
		if name, ok := folderName[sel.ID]; ok {
			base = transfer.SanitizeFilename(name)
		}
	case ScopeAll:
		chosen = all
	default:
		return Package{}, fmt.Errorf("périmètre de partage inconnu : %q", sel.Scope)
	}
	if len(chosen) == 0 {
		return Package{}, errors.New("aucune note à partager")
	}

	var data []byte
	var attCount int
	switch format {
	case FormatMarkdown:
		data, attCount, err = buildMarkdown(ctx, svc, chosen, folders)
	case FormatNoteCore:
		data, attCount, err = buildNoteCore(ctx, svc, chosen, folders, now)
	default:
		err = fmt.Errorf("format de partage inconnu : %q", format)
	}
	if err != nil {
		return Package{}, err
	}

	ext := ".notecore"
	if format == FormatMarkdown {
		ext = ".zip"
	}
	pkg := Package{Data: data, Filename: base + ext, Notes: len(chosen), Attachments: attCount}
	if passphrase != "" {
		if pkg.Data, err = Encrypt(data, passphrase); err != nil {
			return Package{}, err
		}
		pkg.Encrypted = true
	}
	if len(pkg.Data) > MaxPackageBytes {
		return Package{}, fmt.Errorf("paquet trop volumineux (maximum %d Mio)", MaxPackageBytes>>20)
	}
	return pkg, nil
}

func loadAssets(ctx context.Context, svc *notes.Service, noteID string) ([]notes.Attachment, [][]byte, error) {
	list, err := svc.ListAttachments(ctx, noteID)
	if err != nil {
		return nil, nil, err
	}
	blobs := make([][]byte, len(list))
	for i, a := range list {
		d, err := svc.GetAttachmentData(ctx, a.ID)
		if err != nil {
			return nil, nil, err
		}
		if blobs[i], err = base64.StdEncoding.DecodeString(d.DataBase64); err != nil {
			return nil, nil, fmt.Errorf("pièce jointe %q illisible : %w", a.Filename, err)
		}
	}
	return list, blobs, nil
}

func buildMarkdown(ctx context.Context, svc *notes.Service, chosen []notes.Note, folders []notes.Folder) ([]byte, int, error) {
	assets := make(map[string][]transfer.ExportAsset, len(chosen))
	count := 0
	for _, n := range chosen {
		list, blobs, err := loadAssets(ctx, svc, n.ID)
		if err != nil {
			return nil, 0, err
		}
		for i, a := range list {
			assets[n.ID] = append(assets[n.ID], transfer.ExportAsset{ID: a.ID, Filename: a.Filename, Data: blobs[i]})
			count++
		}
	}
	data, err := transfer.ExportAllArchive(chosen, folders, assets)
	return data, count, err
}

func buildNoteCore(ctx context.Context, svc *notes.Service, chosen []notes.Note, folders []notes.Folder, now time.Time) ([]byte, int, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	manifest := Manifest{Format: FormatShare, Version: 1, CreatedAt: now.UTC().Format(time.RFC3339), Count: len(chosen)}
	attCount := 0

	for i, n := range chosen {
		item := Item{
			File:      fmt.Sprintf("notes/%06d.md", i+1),
			Title:     n.Title,
			Tags:      n.Tags,
			Pinned:    n.IsPinned,
			CreatedAt: n.CreatedAt,
			UpdatedAt: n.UpdatedAt,
		}
		if item.Tags == nil {
			item.Tags = []string{}
		}
		if n.FolderID != nil {
			item.FolderPath = transfer.FolderPath(folders, *n.FolderID)
			if len(item.FolderPath) > 0 {
				item.Folder = item.FolderPath[len(item.FolderPath)-1]
			}
		}
		w, err := zw.Create(item.File)
		if err != nil {
			return nil, 0, fmt.Errorf("paquet : %w", err)
		}
		if _, err := w.Write([]byte(n.Content)); err != nil {
			return nil, 0, fmt.Errorf("paquet : %w", err)
		}

		list, blobs, err := loadAssets(ctx, svc, n.ID)
		if err != nil {
			return nil, 0, err
		}
		for j, a := range list {
			file := "attachments/" + a.ID
			aw, err := zw.Create(file)
			if err != nil {
				return nil, 0, fmt.Errorf("paquet : %w", err)
			}
			if _, err := aw.Write(blobs[j]); err != nil {
				return nil, 0, fmt.Errorf("paquet : %w", err)
			}
			item.Attachments = append(item.Attachments, AttItem{ID: a.ID, Filename: a.Filename, Mime: a.Mime, File: file})
			attCount++
		}
		manifest.Notes = append(manifest.Notes, item)
	}

	mw, err := zw.Create("manifest.json")
	if err != nil {
		return nil, 0, fmt.Errorf("paquet : %w", err)
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, 0, err
	}
	if _, err := mw.Write(raw); err != nil {
		return nil, 0, fmt.Errorf("paquet : %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, 0, fmt.Errorf("paquet : %w", err)
	}
	return buf.Bytes(), attCount, nil
}

package share

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	"notecore/backend/internal/notes"
)

// Bornes anti-archive hostile : appliquées AVANT toute écriture.
const (
	maxEntries       = 20000
	maxManifestBytes = 16 << 20
	maxNoteBytes     = 20 << 20
	maxTotalBytes    = 512 << 20
)

// ErrNothing : le fichier ne contient aucune note exploitable.
var ErrNothing = errors.New("aucune note trouvée dans ce fichier")

// Report résume un import, pour l'afficher sans ambiguïté.
type Report struct {
	Imported           int      `json:"imported"`
	Skipped            int      `json:"skipped"` // déjà présentes, à l'identique
	Renamed            int      `json:"renamed"` // même titre, contenu différent : ajoutées à côté
	FoldersCreated     int      `json:"foldersCreated"`
	Attachments        int      `json:"attachments"`
	AttachmentsSkipped int      `json:"attachmentsSkipped"`
	Titles             []string `json:"titles"` // les premiers titres importés
}

type attData struct {
	oldID, filename, mime string
	data                  []byte
}

type draft struct {
	title, content, folder string
	tags                   []string
	pinned                 bool
	atts                   []attData
}

// Import lit un paquet NoteCore, une archive de fichiers Markdown (dont
// l'export de NoteCore) ou un fichier Markdown seul. Il n'écrase, ne
// supprime et ne modifie JAMAIS une note existante.
func Import(ctx context.Context, svc *notes.Service, data []byte, passphrase string) (Report, error) {
	if IsEncrypted(data) {
		plain, err := Decrypt(data, passphrase)
		if err != nil {
			return Report{}, err
		}
		data = plain
	}
	if len(data) > MaxPackageBytes {
		return Report{}, fmt.Errorf("fichier trop volumineux (maximum %d Mio)", MaxPackageBytes>>20)
	}

	var drafts []draft
	var err error
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		drafts, err = readZip(data)
	} else {
		title, content, tags := ParseMarkdown(string(data), "")
		if title == "" || strings.TrimSpace(content) == "" {
			return Report{}, ErrNothing
		}
		drafts = []draft{{title: title, content: content, tags: tags}}
	}
	if err != nil {
		return Report{}, err
	}
	if len(drafts) == 0 {
		return Report{}, ErrNothing
	}
	return apply(ctx, svc, drafts)
}

type budget struct{ left int64 }

func (b *budget) read(f *zip.File, limit int64) ([]byte, error) {
	if int64(f.UncompressedSize64) > limit {
		return nil, fmt.Errorf("%q dépasse la taille autorisée", f.Name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("%q dépasse la taille autorisée", f.Name)
	}
	if b.left -= int64(len(raw)); b.left < 0 {
		return nil, errors.New("archive trop volumineuse une fois décompressée")
	}
	return raw, nil
}

func readZip(data []byte) ([]draft, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("archive illisible : %w", err)
	}
	if len(zr.File) > maxEntries {
		return nil, fmt.Errorf("archive suspecte : %d entrées", len(zr.File))
	}
	byName := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		byName[f.Name] = f
	}
	b := &budget{left: maxTotalBytes}

	if mf, ok := byName["manifest.json"]; ok {
		raw, err := b.read(mf, maxManifestBytes)
		if err != nil {
			return nil, fmt.Errorf("manifeste illisible : %w", err)
		}
		var head struct {
			Format string `json:"format"`
		}
		if json.Unmarshal(raw, &head) == nil && head.Format == FormatShare {
			return readShare(raw, byName, b)
		}
		// Export de NoteCore (notecore-export) : on garde l'association fichier → dossier.
		return readMarkdownZip(zr, b, folderIndex(raw))
	}
	return readMarkdownZip(zr, b, nil)
}

func readShare(raw []byte, byName map[string]*zip.File, b *budget) ([]draft, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("manifeste invalide : %w", err)
	}
	if m.Version != 1 {
		return nil, fmt.Errorf("version de paquet non prise en charge : %d (mettez NoteCore à jour)", m.Version)
	}
	out := make([]draft, 0, len(m.Notes))
	for _, it := range m.Notes {
		f, ok := byName[it.File]
		if !ok {
			return nil, fmt.Errorf("paquet incomplet : %q est absent", it.File)
		}
		content, err := b.read(f, maxNoteBytes)
		if err != nil {
			return nil, err
		}
		d := draft{title: strings.TrimSpace(it.Title), content: string(content), folder: strings.TrimSpace(it.Folder), tags: it.Tags, pinned: it.Pinned}
		for _, a := range it.Attachments {
			af, ok := byName[a.File]
			if !ok {
				d.atts = append(d.atts, attData{oldID: a.ID, filename: a.Filename}) // données absentes : comptée comme ignorée
				continue
			}
			blob, err := b.read(af, notes.MaxAttachmentBytes)
			if err != nil {
				d.atts = append(d.atts, attData{oldID: a.ID, filename: a.Filename})
				continue
			}
			d.atts = append(d.atts, attData{oldID: a.ID, filename: a.Filename, mime: a.Mime, data: blob})
		}
		out = append(out, d)
	}
	return out, nil
}

// folderIndex lit le manifeste d'un export NoteCore et renvoie « nom de
// fichier .md → nom du dossier ».
func folderIndex(raw []byte) map[string]string {
	var m struct {
		Items []struct {
			File   string `json:"file"`
			Folder string `json:"folder"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	out := make(map[string]string, len(m.Items))
	for _, it := range m.Items {
		if it.Folder != "" {
			out[it.File] = it.Folder
		}
	}
	return out
}

func readMarkdownZip(zr *zip.Reader, b *budget, folders map[string]string) ([]draft, error) {
	var out []draft
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		lower := strings.ToLower(name)
		if f.FileInfo().IsDir() || !(strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")) {
			continue
		}
		if strings.HasPrefix(name, "__MACOSX/") || strings.HasPrefix(path.Base(name), ".") {
			continue
		}
		raw, err := b.read(f, maxNoteBytes)
		if err != nil {
			return nil, err
		}
		title, content, tags := ParseMarkdown(string(raw), name)
		if strings.TrimSpace(content) == "" {
			continue
		}
		folder := folders[strings.TrimPrefix(name, "notes/")]
		out = append(out, draft{title: title, content: content, tags: tags, folder: folder})
	}
	return out, nil
}

// importedSuffix complète le titre d'une note importée dont le titre existait déjà.
const importedSuffix = " (importée)"

var attachmentRefRe = regexp.MustCompile(`attachment:[A-Za-z0-9-]+`)

// dupKey identifie une note pour détecter un doublon. Les identifiants de
// pièces jointes sont neutralisés : une note importée en reçoit de nouveaux,
// et le même paquet réimporté doit tout de même être reconnu.
func dupKey(title, content string) string {
	return title + "\x00" + attachmentRefRe.ReplaceAllString(content, notes.AttachmentRefPrefix)
}

// apply écrit les notes. Les doublons exacts (titre et contenu) sont ignorés ;
// une note de même titre mais de contenu différent est ajoutée, son titre
// complété de « (importée) ».
func apply(ctx context.Context, svc *notes.Service, drafts []draft) (Report, error) {
	var rep Report

	existing, err := svc.ListNotes(ctx)
	if err != nil {
		return rep, err
	}
	seenExact := make(map[string]bool, len(existing))
	titles := make(map[string]bool, len(existing))
	for _, n := range existing {
		seenExact[dupKey(n.Title, n.Content)] = true
		// Une note déjà importée sous un titre complété reste reconnue.
		if base, ok := strings.CutSuffix(n.Title, importedSuffix); ok {
			seenExact[dupKey(base, n.Content)] = true
		}
		titles[strings.ToLower(n.Title)] = true
	}
	folders, err := svc.ListFolders(ctx)
	if err != nil {
		return rep, err
	}
	folderID := make(map[string]string, len(folders))
	for _, f := range folders {
		folderID[strings.ToLower(f.Name)] = f.ID
	}

	for _, d := range drafts {
		title := d.title
		if title == "" {
			title = "Note importée"
		}
		if seenExact[dupKey(title, d.content)] {
			rep.Skipped++
			continue
		}
		original := title
		if titles[strings.ToLower(title)] {
			title += importedSuffix
			rep.Renamed++
		}

		fid := ""
		if d.folder != "" {
			key := strings.ToLower(d.folder)
			id, ok := folderID[key]
			if !ok {
				f, err := svc.CreateFolder(ctx, d.folder)
				if err != nil {
					return rep, fmt.Errorf("dossier « %s » : %w", d.folder, err)
				}
				id = f.ID
				folderID[key] = id
				rep.FoldersCreated++
			}
			fid = id
		}

		n, err := svc.CreateNote(ctx, title, fid)
		if err != nil {
			return rep, err
		}
		content := d.content
		for _, a := range d.atts {
			if len(a.data) == 0 {
				rep.AttachmentsSkipped++
				continue
			}
			added, err := svc.AddAttachment(ctx, n.ID, notes.AttachmentUpload{
				Filename: a.filename, Mime: a.mime, DataBase64: base64.StdEncoding.EncodeToString(a.data),
			})
			if err != nil {
				rep.AttachmentsSkipped++
				continue
			}
			content = strings.ReplaceAll(content, notes.AttachmentRefPrefix+a.oldID, notes.AttachmentRefPrefix+added.ID)
			rep.Attachments++
		}
		tags := d.tags
		if tags == nil {
			tags = []string{}
		}
		if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Title: &title, Content: &content, Tags: &tags, IsPinned: &d.pinned}); err != nil {
			// Une note à moitié importée est pire qu'une note absente.
			_ = svc.DeleteNote(ctx, n.ID)
			return rep, err
		}

		seenExact[dupKey(original, content)] = true // le paquet réimporté sera reconnu
		seenExact[dupKey(title, content)] = true
		titles[strings.ToLower(title)] = true
		rep.Imported++
		if len(rep.Titles) < 8 {
			rep.Titles = append(rep.Titles, title)
		}
	}
	return rep, nil
}

// Package transfer porte l'export et la sauvegarde (cdc.md §3.7,
// ADR-004). Doctrine : l'export reste lisible sans NoteCore ; tout
// remplacement de fichier passe par un .tmp puis un renommage
// atomique ; le manifeste du backup est un contrat versionné.
package transfer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"notecore/backend/internal/notes"
)

// FormatExport est le contrat de l'archive multi-notes (ADR-004).
const FormatExport = "notecore-export"

// FormatBackup est le contrat de la sauvegarde complète.
const FormatBackup = "notecore-backup"

// AttachmentScheme préfixe les références de pièces jointes dans le
// Markdown (`attachment:<id>`, ADR-006). À l'export, ces références sont
// réécrites vers un chemin relatif afin que le Markdown reste lisible
// sans NoteCore (cdc.md §3.7).
const AttachmentScheme = "attachment:"

var attachmentRefRe = regexp.MustCompile(`attachment:([A-Za-z0-9-]+)`)

// ExportAsset est une pièce jointe prête à être écrite lors d'un export.
type ExportAsset struct {
	ID       string
	Filename string
	Data     []byte
}

// AttachmentWrite est un fichier de pièce jointe à écrire, chemin
// relatif à la racine de l'export (dossier ou archive).
type AttachmentWrite struct {
	Path string
	Data []byte
}

// ManifestItem décrit une entrée de l'archive d'export.
type ManifestItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	File   string `json:"file"`
	Folder string `json:"folder,omitempty"`
	Tags   string `json:"tags,omitempty"`
}

// Manifest est le contenu de manifest.json — écrit en dernier
// (le manifeste décrit un état terminé, backend-systems).
type Manifest struct {
	Format    string         `json:"format"`
	Version   int            `json:"version"`
	CreatedAt string         `json:"createdAt"`
	Count     int            `json:"count"`
	Items     []ManifestItem `json:"items"`
}

// ExportNoteMarkdown rend une note en Markdown autonome et portable :
// un en-tête front-matter décrit les métadonnées, le contenu suit sans
// transformation (accents, listes et blocs de code intacts — critère
// CDC n°6, §3.7). Les références de pièces jointes sont réécrites via
// `refs` vers leurs chemins relatifs (ADR-006).
func ExportNoteMarkdown(n notes.Note, refs map[string]string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %s\n", n.Title)
	if len(n.Tags) > 0 {
		fmt.Fprintf(&b, "tags: %s\n", strings.Join(n.Tags, ", "))
	}
	fmt.Fprintf(&b, "created: %s\nupdated: %s\n---\n\n", n.CreatedAt, n.UpdatedAt)
	b.WriteString(rewriteAttachmentRefs(n.Content, refs))
	b.WriteString("\n")
	return b.String()
}

// rewriteAttachmentRefs remplace chaque `attachment:<id>` par l'URL
// relative fournie. Une référence inconnue est laissée telle quelle :
// un export ne détruit jamais d'information.
func rewriteAttachmentRefs(content string, refs map[string]string) string {
	if len(refs) == 0 || !strings.Contains(content, AttachmentScheme) {
		return content
	}
	return attachmentRefRe.ReplaceAllStringFunc(content, func(m string) string {
		if url, ok := refs[strings.TrimPrefix(m, AttachmentScheme)]; ok {
			return url
		}
		return m
	})
}

// ExportAttachmentPlan calcule, pour une note, les fichiers de pièces
// jointes à écrire dans un dossier frère `<titre>-assets` et la table de
// correspondance id → URL relative. Les noms sont dédoublonnés et rendus
// sûrs pour une URL Markdown.
func ExportAttachmentPlan(title string, assets []ExportAsset) ([]AttachmentWrite, map[string]string) {
	refs := make(map[string]string, len(assets))
	if len(assets) == 0 {
		return nil, refs
	}
	folder := urlSafeName(SanitizeFilename(title)) + "-assets"
	seen := make(map[string]int, len(assets))
	writes := make([]AttachmentWrite, 0, len(assets))
	for _, a := range assets {
		name := urlSafeName(a.Filename)
		if cnt := seen[name]; cnt > 0 {
			name = fmt.Sprintf("%s-%d", name, cnt+1)
		}
		seen[name]++
		writes = append(writes, AttachmentWrite{Path: folder + "/" + name, Data: a.Data})
		refs[a.ID] = "./" + folder + "/" + name
	}
	return writes, refs
}

// urlSafeName rend un nom utilisable dans une URL Markdown : lettres et
// chiffres conservés (accents compris), le reste devient un tiret.
func urlSafeName(raw string) string {
	var b strings.Builder
	for _, r := range filepath.Base(strings.TrimSpace(raw)) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-.")
	if name == "" {
		name = "fichier"
	}
	if len(name) > 100 {
		ext := filepath.Ext(name)
		if len(ext) > 16 {
			ext = ""
		}
		name = name[:100-len(ext)] + ext
	}
	return name
}

// SanitizeFilename donne un nom de fichier lisible depuis le titre
// sans risque de chemin (pas de séparateur, pas de caractère contrôle).
func SanitizeFilename(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case r == '/', r == '\\', r == ':', r == '"', r == '<', r == '>', r == '|', r == '?', r == '*':
			b.WriteByte('-')
		case unicode.IsPrint(r):
			b.WriteRune(r)
		}
	}
	name := strings.TrimSpace(b.String())
	if name == "" {
		name = "sans-titre"
	}
	return name
}

// ExportAllArchive construit le zip multi-notes : notes/*.md
// dédoublonnées, pièces jointes dans notes/<titre>-assets/, manifeste
// JSON en dernier (il décrit l'archive complète, jamais l'inverse).
func ExportAllArchive(list []notes.Note, assetsByNote map[string][]ExportAsset) ([]byte, error) {
	manifest := Manifest{
		Format:    FormatExport,
		Version:   1,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Count:     len(list),
	}

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	seen := make(map[string]int, len(list))
	for _, n := range list {
		name := SanitizeFilename(n.Title)
		if cnt := seen[name]; cnt > 0 {
			name = fmt.Sprintf("%s-%d", name, cnt+1)
		}
		seen[name]++

		writes, refs := ExportAttachmentPlan(n.Title, assetsByNote[n.ID])

		entry, err := zw.Create("notes/" + name + ".md")
		if err != nil {
			return nil, fmt.Errorf("archive : %w", err)
		}
		if _, err := entry.Write([]byte(ExportNoteMarkdown(n, refs))); err != nil {
			return nil, fmt.Errorf("écriture de %q : %w", name, err)
		}

		for _, w := range writes {
			assetEntry, err := zw.Create("notes/" + w.Path)
			if err != nil {
				return nil, fmt.Errorf("archive (pièce jointe) : %w", err)
			}
			if _, err := assetEntry.Write(w.Data); err != nil {
				return nil, fmt.Errorf("écriture de %q : %w", w.Path, err)
			}
		}

		folder := ""
		if n.FolderID != nil && *n.FolderID != "" {
			folder = *n.FolderID
		}
		manifest.Items = append(manifest.Items, ManifestItem{
			ID:     n.ID,
			Title:  n.Title,
			File:   name + ".md",
			Folder: folder,
			Tags:   strings.Join(n.Tags, ", "),
		})
	}

	entry, err := zw.Create("manifest.json")
	if err != nil {
		return nil, fmt.Errorf("manifeste : %w", err)
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("manifeste JSON : %w", err)
	}
	if _, err := entry.Write(raw); err != nil {
		return nil, fmt.Errorf("écriture manifeste : %w", err)
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("clôture archive : %w", err)
	}
	return buf.Bytes(), nil
}

// SuggestedExportName propose un nom d'archive horodaté et lisible.
func SuggestedExportName() string {
	return fmt.Sprintf("notecore-export-%s.zip", time.Now().Format("2006-01-02"))
}

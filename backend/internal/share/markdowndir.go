package share

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"notecore/backend/internal/notes"
)

// Bornes de l'import d'un dossier de Markdown : un vault Obsidian ou
// Joplin peut être gros, mais l'import reste borné comme une archive —
// les notes hors limite sont comptées ignorées, jamais bloquantes.
const (
	maxDirNotes   = 5000
	maxDirNoteMax = 2 << 20 // 2 Mio par fichier : une note plus grosse est ignorée
)

// ImportMarkdownDir importe un dossier de fichiers .md (vault Obsidian,
// export Joplin, simples notes) : chaque fichier devient une note, la
// hiérarchie des sous-dossiers devient les dossiers NoteCore, le titre
// vient du front-matter, du premier titre ou du nom de fichier. Même
// contrat qu'Import : rien d'existant n'est écrasé ni supprimé.
func ImportMarkdownDir(ctx context.Context, svc *notes.Service, dir string) (Report, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return Report{}, fmt.Errorf("dossier illisible : %w", err)
	}
	if !info.IsDir() {
		return Report{}, errors.New("ce n'est pas un dossier")
	}

	var drafts []draft
	var skipped int
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		name := d.Name()
		if d.IsDir() {
			// Les dossiers cachés (.obsidian, .git, .trash) ne sont pas des notes.
			if strings.HasPrefix(name, ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".md") && !strings.HasSuffix(lower, ".markdown") {
			return nil
		}
		if len(drafts) >= maxDirNotes {
			skipped++
			return nil
		}
		st, err := d.Info()
		if err != nil || st.Size() > maxDirNoteMax {
			skipped++
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			skipped++
			return nil
		}
		title, content, tags := ParseMarkdown(string(raw), name)
		if strings.TrimSpace(content) == "" {
			skipped++
			return nil
		}

		// Le dossier de la note = les sous-dossiers entre la racine
		// choisie et le fichier (la racine elle-même n'est pas un dossier).
		rel, err := filepath.Rel(dir, filepath.Dir(path))
		var folderPath []string
		if err == nil && rel != "." {
			for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
				if part = strings.TrimSpace(part); part != "" {
					folderPath = append(folderPath, part)
				}
			}
		}
		drafts = append(drafts, draft{title: title, content: content, tags: tags, folderPath: folderPath})
		return nil
	})
	if err != nil {
		return Report{}, fmt.Errorf("lecture du dossier : %w", err)
	}
	if len(drafts) == 0 {
		return Report{}, ErrNothing
	}

	rep, err := apply(ctx, svc, drafts)
	if err != nil {
		return rep, err
	}
	rep.Skipped += skipped
	return rep, nil
}

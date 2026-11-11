package share_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"notecore/backend/internal/share"
)

// Import d'un dossier Markdown : chaque .md devient une note, les
// sous-dossiers deviennent des dossiers, les dossiers cachés et les
// fichiers non-Markdown sont ignorés.
func TestImportDossierMarkdown(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("cours/intro.md", "# Introduction\n\nDu texte.")
	write("cours/exos/exo1.md", "# Exercice 1\n\nConsignes.")
	write("idées.md", "---\ntitle: Idée notée\ntags: [projet]\n---\n\nUne idée.")
	write(".obsidian/app.json", "{}")
	write("image.png", "pas du markdown")

	rep, err := share.ImportMarkdownDir(ctx, svc, root)
	if err != nil {
		t.Fatalf("import : %v", err)
	}
	if rep.Imported != 3 {
		t.Errorf("%d notes importées, 3 attendues (rapport %+v)", rep.Imported, rep)
	}
	if rep.FoldersCreated != 2 {
		t.Errorf("%d dossiers créés, 2 attendus (« cours » et « exos »)", rep.FoldersCreated)
	}

	notes, err := svc.ListNotes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 3 {
		t.Fatalf("%d notes en base", len(notes))
	}
	// La note du sous-dossier est rangée sous « cours/exos ».
	folders, err := svc.ListFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]string{}
	for _, f := range folders {
		byName[f.Name] = f.ID
	}
	cours, exos := byName["cours"], byName["exos"]
	if cours == "" || exos == "" {
		t.Fatalf("dossiers manquants : %+v", folders)
	}
	for _, n := range notes {
		switch n.Title {
		case "Introduction":
			if n.FolderID == nil || *n.FolderID != cours {
				t.Errorf("« Introduction » devrait être dans « cours »")
			}
		case "Exercice 1":
			if n.FolderID == nil || *n.FolderID != exos {
				t.Errorf("« Exercice 1 » devrait être dans « cours/exos »")
			}
		case "Idée notée":
			if n.FolderID != nil {
				t.Errorf("« Idée notée » ne devrait avoir aucun dossier")
			}
			if len(n.Tags) != 1 || n.Tags[0] != "projet" {
				t.Errorf("tags front-matter perdus : %+v", n.Tags)
			}
		default:
			t.Errorf("titre inattendu : %q", n.Title)
		}
	}

	// Réimporter le même dossier ne duplique rien.
	rep2, err := share.ImportMarkdownDir(ctx, svc, root)
	if err != nil {
		t.Fatalf("réimport : %v", err)
	}
	if rep2.Imported != 0 || rep2.Skipped != 3 {
		t.Errorf("réimport : %+v — les 3 notes devraient être ignorées", rep2)
	}
}

// Un dossier sans Markdown est refusé proprement.
func TestImportDossierSansMarkdown(t *testing.T) {
	svc := newSvc(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "lu.txt"), []byte("bonjour"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := share.ImportMarkdownDir(context.Background(), svc, root); err != share.ErrNothing {
		t.Errorf("err = %v, ErrNothing attendu", err)
	}
}

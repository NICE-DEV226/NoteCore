package exporter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"notecore/backend/internal/notes"
	"notecore/backend/internal/storage"
)

func newSvc(t *testing.T) *notes.Service {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return notes.New(db)
}

func mustNote(t *testing.T, svc *notes.Service, title, content string) notes.Note {
	t.Helper()
	n, err := svc.CreateNote(context.Background(), title, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateNote(context.Background(), n.ID, notes.Patch{Content: &content}); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSyncEcritLesNotes(t *testing.T) {
	svc := newSvc(t)
	mustNote(t, svc, "Cours algo", "# Intro\n\nDu contenu.")
	dir := t.TempDir()

	res, err := New(svc).Sync(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Synced || res.Files != 1 {
		t.Fatalf("résultat inattendu : %+v", res)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "Cours algo.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `title: "Cours algo"`) || !strings.Contains(body, "Du contenu.") {
		t.Fatalf("front-matter ou contenu absent : %q", body)
	}
	if _, err := os.Stat(filepath.Join(dir, ManifestName)); err != nil {
		t.Fatal("manifeste absent")
	}
}

func TestSyncMiroirDossiers(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	f, err := svc.CreateFolder(ctx, "Maths")
	if err != nil {
		t.Fatal(err)
	}
	n, err := svc.CreateNote(ctx, "Intégrales", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: strPtrT("∫ x dx")}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if _, err := New(svc).Sync(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Maths", "Intégrales.md")); err != nil {
		t.Fatalf("note non exportée dans son dossier : %v", err)
	}
}

func TestSyncSansDossier(t *testing.T) {
	res, err := New(newSvc(t)).Sync(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Synced {
		t.Fatal("sans dossier configuré, rien ne doit être synchronisé")
	}
}

func TestSyncIncrementalEtNettoyage(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	a := mustNote(t, svc, "Alpha", "un")
	b := mustNote(t, svc, "Bêta", "deux")
	dir := t.TempDir()
	s := New(svc)

	if _, err := s.Sync(ctx, dir); err != nil {
		t.Fatal(err)
	}

	// Modification : le fichier est réécrit, le reste intact.
	if _, err := svc.UpdateNote(ctx, a.ID, notes.Patch{Content: strPtrT("un bis")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(ctx, dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "Alpha.md"))
	if !strings.Contains(string(raw), "un bis") {
		t.Fatal("modification non répercutée")
	}

	// Suppression : le fichier disparaît, celui du voisin reste.
	if err := svc.DeleteNoteForever(ctx, b.ID); err != nil {
		// DeleteNoteForever exige la corbeille d'abord.
		if err := svc.DeleteNote(ctx, b.ID); err != nil {
			t.Fatal(err)
		}
		if err := svc.DeleteNoteForever(ctx, b.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Sync(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Bêta.md")); !os.IsNotExist(err) {
		t.Fatal("fichier de la note supprimée toujours présent")
	}
	if _, err := os.Stat(filepath.Join(dir, "Alpha.md")); err != nil {
		t.Fatal("fichier de la note conservée disparu")
	}
}

func TestSyncNeTouchePasAuxEtrangers(t *testing.T) {
	svc := newSvc(t)
	mustNote(t, svc, "Note", "x")
	dir := t.TempDir()
	foreign := filepath.Join(dir, "mes-propres-notes.md")
	if err := os.WriteFile(foreign, []byte("à moi"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(svc)
	if _, err := s.Sync(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(foreign)
	if err != nil || string(raw) != "à moi" {
		t.Fatal("un fichier .md étranger a été touché")
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"Cours/été":                             "Cours-été",
		"  espaces  ":                           "espaces",
		"...":                                   "sans-titre",
		"a*b?c":                                 "a-b-c",
		"très long " + strings.Repeat("x", 100): "très long " + strings.Repeat("x", 70),
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, attendu %q", in, got, want)
		}
	}
}

func strPtrT(s string) *string { return &s }

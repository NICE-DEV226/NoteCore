package share_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"notecore/backend/internal/notes"
	"notecore/backend/internal/share"
	"notecore/backend/internal/storage"
	"notecore/backend/internal/transfer"
)

func newSvc(t *testing.T) *notes.Service {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return notes.New(db)
}

// seed crée un dossier, une note avec étiquettes, épinglée, et une image jointe.
func seed(t *testing.T, svc *notes.Service) (notes.Note, notes.Attachment) {
	t.Helper()
	ctx := context.Background()
	f, err := svc.CreateFolder(ctx, "Réseaux")
	if err != nil {
		t.Fatal(err)
	}
	n, err := svc.CreateNote(ctx, "Modèle OSI", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1a\nfaux-pixels")
	att, err := svc.AddAttachment(ctx, n.ID, notes.AttachmentUpload{
		Filename: "schema.png", Mime: "image/png", DataBase64: base64.StdEncoding.EncodeToString(png),
	})
	if err != nil {
		t.Fatal(err)
	}
	content := "# OSI\n\nSept couches. Le {{c1::transport}} est la 4e.\n\n![schéma](attachment:" + att.ID + ")\n"
	tags := []string{"réseaux", "cours"}
	pinned := true
	n, err = svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &content, Tags: &tags, IsPinned: &pinned})
	if err != nil {
		t.Fatal(err)
	}
	return n, att
}

func TestChiffrement(t *testing.T) {
	plain := []byte("contenu confidentiel")
	enc, err := share.Encrypt(plain, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !share.IsEncrypted(enc) || bytes.Contains(enc, plain) {
		t.Fatal("le paquet doit être chiffré")
	}
	got, err := share.Decrypt(enc, "correct horse")
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("aller-retour : %q, %v", got, err)
	}
	if _, err := share.Decrypt(enc, "mauvais"); !errors.Is(err, share.ErrBadPassphrase) {
		t.Errorf("mauvais mot de passe : %v", err)
	}
	if _, err := share.Decrypt(enc, ""); !errors.Is(err, share.ErrPassphraseRequired) {
		t.Errorf("mot de passe absent : %v", err)
	}
	enc[len(enc)-1] ^= 0xFF
	if _, err := share.Decrypt(enc, "correct horse"); !errors.Is(err, share.ErrBadPassphrase) {
		t.Errorf("paquet altéré : %v", err)
	}
	// Deux chiffrements du même texte diffèrent (sel et nonce aléatoires).
	a, _ := share.Encrypt(plain, "x")
	b, _ := share.Encrypt(plain, "x")
	if bytes.Equal(a, b) {
		t.Error("sel et nonce doivent être aléatoires")
	}
}

func TestParseMarkdown(t *testing.T) {
	cases := []struct {
		name, raw, file, title string
		tags                   []string
		contentHas             string
	}{
		{"en-tête NoteCore", "---\ntitle: \"Cours: Go # 1\"\ntags: [a, \"b,c\"]\ncreated: x\nupdated: y\n---\n\nCorps\n", "x.md", "Cours: Go # 1", []string{"a", "b,c"}, "Corps"},
		{"liste à tirets", "---\ntitle: T\ntags:\n  - un\n  - deux\n---\ntexte", "x.md", "T", []string{"un", "deux"}, "texte"},
		{"premier titre", "\n# Mon titre\n\nCorps", "x.md", "Mon titre", nil, "Corps"},
		{"nom de fichier", "juste du texte", "dossier/ma note.md", "ma note", nil, "juste du texte"},
		{"BOM et CRLF", "\xef\xbb\xbf---\r\ntitle: Windows\r\n---\r\nligne\r\n", "x.md", "Windows", nil, "ligne"},
	}
	for _, c := range cases {
		title, content, tags := share.ParseMarkdown(c.raw, c.file)
		if title != c.title || !strings.Contains(content, c.contentHas) || fmt.Sprint(tags) != fmt.Sprint(c.tags) {
			t.Errorf("%s : titre=%q tags=%v contenu=%q", c.name, title, tags, content)
		}
		if strings.Contains(content, "---") && strings.HasPrefix(c.raw, "---") {
			t.Errorf("%s : l'en-tête ne doit pas rester dans le contenu : %q", c.name, content)
		}
	}
}

func TestPaquetNoteCoreAllerRetour(t *testing.T) {
	ctx := context.Background()
	src, dst := newSvc(t), newSvc(t)
	n, att := seed(t, src)

	pkg, err := share.Build(ctx, src, share.Selection{Scope: share.ScopeNote, ID: n.ID}, share.FormatNoteCore, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Notes != 1 || pkg.Attachments != 1 || pkg.Filename != "Modèle OSI.notecore" || pkg.Encrypted {
		t.Errorf("paquet = %+v", pkg)
	}

	rep, err := share.Import(ctx, dst, pkg.Data, "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Imported != 1 || rep.FoldersCreated != 1 || rep.Attachments != 1 || rep.Skipped != 0 {
		t.Errorf("rapport = %+v", rep)
	}

	list, _ := dst.ListNotes(ctx)
	if len(list) != 1 {
		t.Fatalf("1 note attendue, %d", len(list))
	}
	got := list[0]
	if got.Title != "Modèle OSI" || !got.IsPinned || len(got.Tags) != 2 || got.FolderID == nil {
		t.Errorf("note importée = %+v", got)
	}
	folders, _ := dst.ListFolders(ctx)
	if len(folders) != 1 || folders[0].Name != "Réseaux" {
		t.Errorf("dossier = %+v", folders)
	}

	// La pièce jointe a un NOUVEL identifiant ; le texte pointe vers lui, pas vers l'ancien.
	atts, _ := dst.ListAttachments(ctx, got.ID)
	if len(atts) != 1 || atts[0].ID == att.ID || atts[0].Filename != "schema.png" {
		t.Fatalf("pièces jointes = %+v", atts)
	}
	if strings.Contains(got.Content, att.ID) || !strings.Contains(got.Content, "attachment:"+atts[0].ID) {
		t.Errorf("références non réécrites : %q", got.Content)
	}
	data, err := dst.GetAttachmentData(ctx, atts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(data.DataBase64)
	if !bytes.HasPrefix(raw, []byte("\x89PNG")) {
		t.Error("octets de la pièce jointe altérés")
	}
}

func TestImportNeDetruitRien(t *testing.T) {
	ctx := context.Background()
	src, dst := newSvc(t), newSvc(t)
	n, _ := seed(t, src)
	pkg, err := share.Build(ctx, src, share.Selection{Scope: share.ScopeAll}, share.FormatNoteCore, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// L'utilisateur a déjà une note de même titre, au contenu différent.
	mine, _ := dst.CreateNote(ctx, n.Title, "")
	text := "mon propre texte, à ne jamais écraser"
	if _, err := dst.UpdateNote(ctx, mine.ID, notes.Patch{Content: &text}); err != nil {
		t.Fatal(err)
	}

	rep, err := share.Import(ctx, dst, pkg.Data, "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Imported != 1 || rep.Renamed != 1 {
		t.Errorf("rapport = %+v", rep)
	}
	kept, _ := dst.GetNote(ctx, mine.ID)
	if kept.Content != text || kept.Title != n.Title {
		t.Errorf("la note existante a été modifiée : %+v", kept)
	}
	all, _ := dst.ListNotes(ctx)
	if len(all) != 2 {
		t.Fatalf("2 notes attendues, %d", len(all))
	}

	// Réimporter le même paquet n'ajoute rien.
	rep, err = share.Import(ctx, dst, pkg.Data, "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Imported != 0 || rep.Skipped != 1 {
		t.Errorf("réimport = %+v", rep)
	}
	if all, _ = dst.ListNotes(ctx); len(all) != 2 {
		t.Errorf("le réimport a créé des doublons : %d notes", len(all))
	}
}

func TestPaquetProtegeParMotDePasse(t *testing.T) {
	ctx := context.Background()
	src, dst := newSvc(t), newSvc(t)
	seed(t, src)
	pkg, err := share.Build(ctx, src, share.Selection{Scope: share.ScopeAll}, share.FormatNoteCore, "sésame", time.Now())
	if err != nil || !pkg.Encrypted {
		t.Fatalf("paquet chiffré attendu : %v %+v", err, pkg.Encrypted)
	}
	if _, err := share.Import(ctx, dst, pkg.Data, ""); !errors.Is(err, share.ErrPassphraseRequired) {
		t.Errorf("sans mot de passe : %v", err)
	}
	if _, err := share.Import(ctx, dst, pkg.Data, "faux"); !errors.Is(err, share.ErrBadPassphrase) {
		t.Errorf("mauvais mot de passe : %v", err)
	}
	if list, _ := dst.ListNotes(ctx); len(list) != 0 {
		t.Error("un import refusé ne doit rien écrire")
	}
	if rep, err := share.Import(ctx, dst, pkg.Data, "sésame"); err != nil || rep.Imported != 1 {
		t.Errorf("avec le bon mot de passe : %+v %v", rep, err)
	}
	if _, err := share.Build(ctx, src, share.Selection{Scope: share.ScopeAll}, share.FormatMarkdown, "x", time.Now()); err == nil {
		t.Error("le mot de passe n'a pas de sens pour l'archive Markdown")
	}
}

func TestImportArchiveMarkdownDeNoteCore(t *testing.T) {
	ctx := context.Background()
	src, dst := newSvc(t), newSvc(t)
	seed(t, src)
	pkg, err := share.Build(ctx, src, share.Selection{Scope: share.ScopeAll}, share.FormatMarkdown, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(pkg.Filename, ".zip") {
		t.Errorf("nom = %q", pkg.Filename)
	}
	rep, err := share.Import(ctx, dst, pkg.Data, "")
	if err != nil || rep.Imported != 1 || rep.FoldersCreated != 1 {
		t.Fatalf("rapport = %+v, err = %v", rep, err)
	}
	list, _ := dst.ListNotes(ctx)
	if list[0].Title != "Modèle OSI" || len(list[0].Tags) != 2 {
		t.Errorf("note = %+v", list[0])
	}
	if strings.HasPrefix(list[0].Content, "---") {
		t.Error("l'en-tête YAML ne doit pas se retrouver dans le texte")
	}
}

func TestImportFichiersMarkdownDivers(t *testing.T) {
	ctx := context.Background()
	dst := newSvc(t)

	// Un fichier Markdown seul.
	if rep, err := share.Import(ctx, dst, []byte("# Rapport\n\ncorps"), ""); err != nil || rep.Imported != 1 {
		t.Fatalf("fichier seul : %+v %v", rep, err)
	}
	// Un zip d'un autre outil : sous-dossiers, fichiers cachés, __MACOSX, non-Markdown.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"vault/a.md":          "# A\ntexte a",
		"vault/sub/b.md":      "---\ntitle: B\ntags: x\n---\ntexte b",
		"vault/.obsidian.md":  "caché",
		"__MACOSX/vault/c.md": "poubelle",
		"vault/image.png":     "pas du markdown",
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	rep, err := share.Import(ctx, dst, buf.Bytes(), "")
	if err != nil || rep.Imported != 2 {
		t.Fatalf("zip : %+v %v", rep, err)
	}
	if _, err := share.Import(ctx, dst, []byte("   \n"), ""); !errors.Is(err, share.ErrNothing) {
		t.Errorf("fichier vide : %v", err)
	}
}

func TestImportRefuseArchiveHostile(t *testing.T) {
	ctx := context.Background()
	dst := newSvc(t)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i < 20001; i++ {
		zw.Create(fmt.Sprintf("n%d.md", i))
	}
	zw.Close()
	if _, err := share.Import(ctx, dst, buf.Bytes(), ""); err == nil {
		t.Error("une archive de 20 001 entrées doit être refusée")
	}

	// Un paquet d'une version plus récente est refusé avec un message clair.
	buf.Reset()
	zw = zip.NewWriter(&buf)
	w, _ := zw.Create("manifest.json")
	w.Write([]byte(`{"format":"notecore-share","version":99,"notes":[]}`))
	zw.Close()
	if _, err := share.Import(ctx, dst, buf.Bytes(), ""); err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("version inconnue : %v", err)
	}

	// Un paquet dont une note manque n'est pas à moitié importé.
	buf.Reset()
	zw = zip.NewWriter(&buf)
	w, _ = zw.Create("manifest.json")
	w.Write([]byte(`{"format":"notecore-share","version":1,"notes":[{"file":"notes/000001.md","title":"X"}]}`))
	zw.Close()
	if _, err := share.Import(ctx, dst, buf.Bytes(), ""); err == nil {
		t.Error("paquet incomplet accepté")
	}
	if list, _ := dst.ListNotes(ctx); len(list) != 0 {
		t.Error("aucune note ne doit être écrite par un import refusé")
	}
}

func TestPerimetres(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	n, _ := seed(t, svc)
	other, _ := svc.CreateNote(ctx, "Hors dossier", "")

	for _, c := range []struct {
		sel   share.Selection
		count int
		name  string
	}{
		{share.Selection{Scope: share.ScopeNote, ID: other.ID}, 1, "Hors dossier.notecore"},
		{share.Selection{Scope: share.ScopeFolder, ID: *n.FolderID}, 1, "Réseaux.notecore"},
	} {
		pkg, err := share.Build(ctx, svc, c.sel, share.FormatNoteCore, "", time.Now())
		if err != nil || pkg.Notes != c.count || pkg.Filename != c.name {
			t.Errorf("%+v : %+v %v", c.sel, pkg.Filename, err)
		}
	}
	if pkg, _ := share.Build(ctx, svc, share.Selection{Scope: share.ScopeAll}, share.FormatNoteCore, "", time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)); pkg.Notes != 2 || pkg.Filename != "notecore-notes-2026-10-20.notecore" {
		t.Errorf("tout : %+v", pkg.Filename)
	}
	if _, err := share.Build(ctx, svc, share.Selection{Scope: share.ScopeNote, ID: "inconnue"}, share.FormatNoteCore, "", time.Now()); err == nil {
		t.Error("une note inconnue doit être refusée")
	}
	_ = transfer.FormatExport
}

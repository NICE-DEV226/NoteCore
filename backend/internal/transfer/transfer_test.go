// Tests du module transfer : export portable, archive/manifeste,
// backup avec empreinte et détection de corruption.
package transfer

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"notecore/backend/internal/notes"
	"notecore/backend/internal/storage"
)

func makeDB(t *testing.T) (*sql.DB, *notes.Service) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, notes.New(db)
}

// L'export reste lisible sans NoteCore et conserve le contenu à
// l'identique (critères CDC n°6 et 7 : accents, code, listes, Markdown).
func TestExportMarkdownPortable(t *testing.T) {
	note := notes.Note{
		ID:      "n-1",
		Title:   "Réseaux — câble & contrôle",
		Content: "```c\nint main(void) {\n  return 0;\n}\n```",
		Tags:    []string{"réseaux", "c"},
	}
	out := ExportNoteMarkdown(note, nil)

	cases := [][2]string{
		{"front-matter", "title: Réseaux — câble & contrôle\n"},
		{"bloc de code intact", "int main(void) {\n  return 0;\n}"},
		{"accent", "réseaux"},
		{"contenu non transformé", "```c"},
	}
	for _, tc := range cases {
		if !strings.Contains(out, tc[1]) {
			t.Errorf("%s absent de l'export", tc[0])
		}
	}
}

// Les noms de fichiers sont sûrs (titre transformé, pas de chemin).
func TestSanitizeFilename(t *testing.T) {
	if got := SanitizeFilename("a/b\\c:d*e?f"); strings.ContainsAny(got, "/\\:*?") {
		t.Errorf("nom non sécurisé : %q", got)
	}
	if got := SanitizeFilename("   "); got != "sans-titre" {
		t.Errorf("titre vide = %q, attendu sans-titre", got)
	}
	if got := SanitizeFilename("Accents éàç"); !strings.Contains(got, "éàç") {
		t.Errorf("accents perdus : %q", got)
	}
}

// L'archive contient toutes les notes, des noms dédoublonnés et un
// manifeste conforme au contrat (ADR-004).
func TestArchiveExport(t *testing.T) {
	list := []notes.Note{
		{ID: "n-1", Title: "Note", Content: "a", Tags: []string{"x"}},
		{ID: "n-2", Title: "Note", Content: "b"},
		{ID: "n-3", Title: "Instrument", Content: "c"},
	}
	data, err := ExportAllArchive(list, nil)
	if err != nil {
		t.Fatal(err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	nameCount := 0
	manifestCount := 0
	var manifestBytes []byte
	for _, f := range zr.File {
		switch {
		case f.Name == "manifest.json":
			manifestCount++
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			manifestBytes, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
		case strings.HasPrefix(f.Name, "notes/"):
			nameCount++
		}
	}
	if nameCount != 3 {
		t.Errorf("3 notes attendues dans notes/, obtenu %d", nameCount)
	}
	if manifestCount != 1 {
		t.Fatalf("manifest.json absent ou dupliqué (%d)", manifestCount)
	}

	var m Manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		t.Fatalf("manifeste illisible : %v", err)
	}
	if m.Format != FormatExport || m.Version != 1 {
		t.Errorf("contrat de manifeste erroné : %s v%d", m.Format, m.Version)
	}
	if m.Count != 3 || len(m.Items) != 3 {
		t.Errorf("comptage du manifeste erroné : count=%d items=%d", m.Count, len(m.Items))
	}

	// Dédoublonnage : deux notes « Note » → Note.md + Note-2.md
	if !strings.Contains(readEntry(t, zr, "notes/Note-2.md"), "b") {
		t.Error("le contenu de la seconde note dédoublonnée a été perdu")
	}
}

// Les références de pièces jointes sont réécrites vers des chemins
// relatifs, et une référence inconnue est laissée intacte (ADR-006 :
// un export ne détruit jamais d'information).
func TestExportReecritPiecesJointes(t *testing.T) {
	note := notes.Note{
		ID:      "n-1",
		Title:   "Photos de terrain",
		Content: "Avant ![vue](attachment:aaa-1) et après ![plan](attachment:bbb-2).",
	}
	assets := []ExportAsset{
		{ID: "aaa-1", Filename: "vue éclatée.png", Data: []byte("PNG1")},
		{ID: "bbb-2", Filename: "plan.jpg", Data: []byte("JPG2")},
	}
	writes, refs := ExportAttachmentPlan(note.Title, assets)
	if len(writes) != 2 {
		t.Fatalf("2 fichiers attendus, obtenu %d", len(writes))
	}
	// Le dossier suit le titre, les accents/noms restent lisibles.
	if writes[0].Path != "Photos-de-terrain-assets/vue-éclatée.png" {
		t.Errorf("chemin inattendu : %q", writes[0].Path)
	}
	out := ExportNoteMarkdown(note, refs)
	if !strings.Contains(out, "![vue](./Photos-de-terrain-assets/vue-éclatée.png)") {
		t.Errorf("référence image non réécrite :\n%s", out)
	}
	if !strings.Contains(out, "![plan](./Photos-de-terrain-assets/plan.jpg)") {
		t.Errorf("référence image non réécrite :\n%s", out)
	}
	// Référence non résolue : conservée telle quelle.
	orphan := ExportNoteMarkdown(notes.Note{Content: "![x](attachment:inconnu)"}, refs)
	if !strings.Contains(orphan, "attachment:inconnu") {
		t.Errorf("une référence inconnue doit rester intacte : %q", orphan)
	}
}

// L'archive range les pièces jointes dans un dossier frère du Markdown.
func TestArchiveAvecPiecesJointes(t *testing.T) {
	list := []notes.Note{
		{ID: "n-1", Title: "Terrain", Content: "![v](attachment:aaa-1)"},
	}
	assets := map[string][]ExportAsset{
		"n-1": {{ID: "aaa-1", Filename: "vue.png", Data: []byte("PNGDATA")}},
	}
	data, err := ExportAllArchive(list, assets)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got := readEntry(t, zr, "notes/Terrain-assets/vue.png"); got != "PNGDATA" {
		t.Errorf("octets de la pièce jointe altérés : %q", got)
	}
	if md := readEntry(t, zr, "notes/Terrain.md"); !strings.Contains(md, "./Terrain-assets/vue.png") {
		t.Errorf("le Markdown archivé ne pointe pas vers sa pièce jointe :\n%s", md)
	}
}

// Roundtrip complet du backup : création, manifeste, empreinte, puis
// détection de corruption (CDC critère n°8 : aucune restauration
// silencieuse de données altérées).
func TestBackupEtValidation(t *testing.T) {
	db, svc := makeDB(t)
	ctx := context.Background()

	if _, err := svc.CreateNote(ctx, "note de contrôle", ""); err != nil {
		t.Fatal(err)
	}
	schemaVersion, err := storage.SchemaVersion(db)
	if err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "sauvegarde.notecore.zip")
	if err := CreateBackup(db, schemaVersion, dest); err != nil {
		t.Fatalf("backup : %v", err)
	}

	manifest, err := ValidateBackup(dest)
	if err != nil {
		t.Fatalf("validation : %v", err)
	}
	if manifest.Format != FormatBackup || manifest.SchemaVersion != schemaVersion {
		t.Errorf("manifeste incohérent : %+v", manifest)
	}
	if manifest.SHA256 == "" || manifest.Size <= 0 {
		t.Error("empreinte absente")
	}

	// Corruption d'un octet : la validation doit détecter l'anomalie.
	corrupted := filepath.Join(t.TempDir(), "corrompu.notecore.zip")
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 0xFF
	if err := os.WriteFile(corrupted, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateBackup(corrupted); err == nil {
		t.Error("corruption non détectée par l'empreinte SHA-256")
	}
}

func TestRestoreBackupRoundtrip(t *testing.T) {
	db, svc := makeDB(t)
	ctx := context.Background()
	created, err := svc.CreateNote(ctx, "note restaurée", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateNote(ctx, created.ID, notes.Patch{Content: stringPtr("contenu sauvegardé")}); err != nil {
		t.Fatal(err)
	}
	// Une pièce jointe doit survivre au cycle sauvegarde/restauration
	// (ADR-006 : les octets vivent dans la base, donc dans la sauvegarde).
	raw := []byte("contenu binaire de la pièce jointe")
	att, err := svc.AddAttachment(ctx, created.ID, notes.AttachmentUpload{
		Filename:   "annexe.bin",
		Mime:       "application/octet-stream",
		DataBase64: base64.StdEncoding.EncodeToString(raw),
	})
	if err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "roundtrip.notecore.zip")
	schemaVersion, err := storage.SchemaVersion(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateBackup(db, schemaVersion, dest); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteNote(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := RestoreBackup(db, schemaVersion, dest); err != nil {
		t.Fatalf("restauration : %v", err)
	}
	got, err := svc.GetNote(ctx, created.ID)
	if err != nil {
		t.Fatalf("lecture après restauration : %v", err)
	}
	if got.Content != "contenu sauvegardé" {
		t.Errorf("contenu restauré = %q", got.Content)
	}
	data, err := svc.GetAttachmentData(ctx, att.ID)
	if err != nil {
		t.Fatalf("pièce jointe perdue à la restauration : %v", err)
	}
	back, err := base64.StdEncoding.DecodeString(data.DataBase64)
	if err != nil || string(back) != string(raw) {
		t.Errorf("octets restaurés altérés : %v (%q)", err, back)
	}
}

// Le manifeste est un contrat : une empreinte absente est refusée
// (aucune restauration sans contrôle d'intégrité — ADR-004).
func TestValidateBackupRefuseSansEmpreinte(t *testing.T) {
	dest := makeBackup(t)
	forged := filepath.Join(t.TempDir(), "sans-sha.zip")
	rewriteManifest(t, dest, forged, func(m *BackupManifest) { m.SHA256 = "" })
	if _, err := ValidateBackup(forged); err == nil {
		t.Error("empreinte absente acceptée")
	}
}

// Une version de conteneur inconnue (plus récente que l'application) est
// refusée : on ne devine pas un format qu'on ne sait pas lire.
func TestValidateBackupRefuseVersionInconnue(t *testing.T) {
	dest := makeBackup(t)
	forged := filepath.Join(t.TempDir(), "version.zip")
	rewriteManifest(t, dest, forged, func(m *BackupManifest) { m.Version = 999 })
	if _, err := ValidateBackup(forged); err == nil {
		t.Error("version inconnue acceptée")
	}
}

// Une taille annoncée qui ne correspond pas au contenu est une
// incohérence : refus avant toute extraction.
func TestValidateBackupRefuseTailleDivergente(t *testing.T) {
	dest := makeBackup(t)
	forged := filepath.Join(t.TempDir(), "taille.zip")
	rewriteManifest(t, dest, forged, func(m *BackupManifest) { m.Size = m.Size + 4096 })
	if _, err := ValidateBackup(forged); err == nil {
		t.Error("taille divergente acceptée")
	}
}

// Restaurer une sauvegarde issue d'un schéma plus récent détruirait des
// colonnes inconnues : la restauration est refusée.
func TestRestoreBackupRefuseSchemaPlusRecent(t *testing.T) {
	db, svc := makeDB(t)
	ctx := context.Background()
	if _, err := svc.CreateNote(ctx, "témoin", ""); err != nil {
		t.Fatal(err)
	}
	schemaVersion, err := storage.SchemaVersion(db)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "recent.zip")
	if err := CreateBackup(db, schemaVersion, dest); err != nil {
		t.Fatal(err)
	}

	forged := filepath.Join(t.TempDir(), "nouveau-schema.zip")
	rewriteManifest(t, dest, forged, func(m *BackupManifest) { m.SchemaVersion = schemaVersion + 5 })

	if err := RestoreBackup(db, schemaVersion, forged); err == nil {
		t.Error("restauration d'un schéma plus récent acceptée")
	}
}

// makeBackup crée une sauvegarde valide d'une base contenant une note.
func makeBackup(t *testing.T) string {
	t.Helper()
	db, svc := makeDB(t)
	if _, err := svc.CreateNote(context.Background(), "note", ""); err != nil {
		t.Fatal(err)
	}
	schemaVersion, err := storage.SchemaVersion(db)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "backup.zip")
	if err := CreateBackup(db, schemaVersion, dest); err != nil {
		t.Fatal(err)
	}
	return dest
}

// rewriteManifest recopie une archive en remplaçant son manifeste après
// mutation, sans toucher à la base : l'empreinte (calculée sur la base)
// reste donc valide, ce qui isole la règle testée.
func rewriteManifest(t *testing.T, src, dst string, mutate func(*BackupManifest)) {
	t.Helper()
	zr, err := zip.OpenReader(src)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	zw := zip.NewWriter(out)

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if f.Name == "manifest.json" {
			var m BackupManifest
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatal(err)
			}
			mutate(&m)
			if data, err = json.Marshal(&m); err != nil {
				t.Fatal(err)
			}
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func stringPtr(value string) *string {
	return &value
}

// --- helpers locaux -----------------------------------------------------

func readEntry(t *testing.T, zr *zip.Reader, name string) string {
	t.Helper()
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				t.Fatal(err)
			}
			return string(data)
		}
	}
	t.Fatalf("entrée %q introuvable", name)
	return ""
}

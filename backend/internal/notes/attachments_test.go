package notes_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"notecore/backend/internal/notes"
)

func addNote(t *testing.T, svc *notes.Service, title string) notes.Note {
	t.Helper()
	n, err := svc.CreateNote(context.Background(), title, "")
	if err != nil {
		t.Fatalf("création note : %v", err)
	}
	return n
}

// Aller-retour complet d'une pièce jointe : ajout, listage, lecture des
// octets à l'identique, suppression (ADR-006).
func TestPieceJointeAllerRetour(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	n := addNote(t, svc, "avec pièce jointe")
	raw := []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0xff}

	a, err := svc.AddAttachment(ctx, n.ID, notes.AttachmentUpload{
		Filename:   "vue.png",
		Mime:       "image/png",
		DataBase64: base64.StdEncoding.EncodeToString(raw),
	})
	if err != nil {
		t.Fatalf("ajout pièce jointe : %v", err)
	}
	if a.Size != int64(len(raw)) || a.Mime != "image/png" || a.Filename != "vue.png" {
		t.Errorf("métadonnées erronées : %+v", a)
	}

	list, err := svc.ListAttachments(ctx, n.ID)
	if err != nil || len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("listage erroné : %v (list=%v)", err, list)
	}

	data, err := svc.GetAttachmentData(ctx, a.ID)
	if err != nil {
		t.Fatalf("lecture pièce jointe : %v", err)
	}
	back, err := base64.StdEncoding.DecodeString(data.DataBase64)
	if err != nil || string(back) != string(raw) {
		t.Errorf("octets altérés : %v (reçu %v)", err, back)
	}

	if err := svc.DeleteAttachment(ctx, a.ID); err != nil {
		t.Fatalf("suppression pièce jointe : %v", err)
	}
	if list, _ = svc.ListAttachments(ctx, n.ID); len(list) != 0 {
		t.Errorf("la pièce jointe survit à sa suppression : %v", list)
	}
}

// Supprimer une note emporte ses pièces jointes : aucune ligne orpheline
// (clé étrangère ON DELETE CASCADE, ADR-006).
func TestPieceJointeSupprimeeAvecLaNote(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	n := addNote(t, svc, "à supprimer")

	a, err := svc.AddAttachment(ctx, n.ID, notes.AttachmentUpload{
		Filename:   "note.txt",
		Mime:       "text/plain",
		DataBase64: base64.StdEncoding.EncodeToString([]byte("contenu")),
	})
	if err != nil {
		t.Fatalf("ajout : %v", err)
	}

	if err := svc.DeleteNote(ctx, n.ID); err != nil {
		t.Fatalf("suppression note : %v", err)
	}
	if _, err := svc.GetAttachmentData(ctx, a.ID); err == nil {
		t.Error("la pièce jointe survit à la suppression de sa note")
	}
}

// Une pièce jointe trop volumineuse est refusée avant tout décodage.
func TestPieceJointeTropVolumineuse(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	n := addNote(t, svc, "limite")

	tooBig := strings.Repeat("A", base64.StdEncoding.EncodedLen(int(notes.MaxAttachmentBytes))+8)
	if _, err := svc.AddAttachment(ctx, n.ID, notes.AttachmentUpload{
		Filename:   "gros.bin",
		Mime:       "application/octet-stream",
		DataBase64: tooBig,
	}); err == nil {
		t.Fatal("une pièce jointe au-delà de la borne doit être refusée")
	}
}

// Une pièce jointe ne peut pas viser une note inexistante (intégrité).
func TestPieceJointeNoteInexistante(t *testing.T) {
	svc := newService(t)
	if _, err := svc.AddAttachment(context.Background(), "note-absente", notes.AttachmentUpload{
		Filename:   "x.txt",
		Mime:       "text/plain",
		DataBase64: base64.StdEncoding.EncodeToString([]byte("x")),
	}); err == nil {
		t.Fatal("une note inexistante doit être refusée")
	}
}

// Le nom conservé ne contient jamais de chemin ni de caractère de
// contrôle : il ne sert qu'à l'affichage et à l'export.
func TestNomPieceJointeAssaini(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	n := addNote(t, svc, "noms")

	a, err := svc.AddAttachment(ctx, n.ID, notes.AttachmentUpload{
		Filename:   "../../etc/pa\x00sswd",
		Mime:       "text/plain",
		DataBase64: base64.StdEncoding.EncodeToString([]byte("x")),
	})
	if err != nil {
		t.Fatalf("ajout : %v", err)
	}
	if strings.ContainsAny(a.Filename, "/\\") || strings.ContainsRune(a.Filename, 0) {
		t.Errorf("nom non assaini : %q", a.Filename)
	}
}

// Le type d'une pièce jointe est déduit du contenu puis de l'extension
// quand le navigateur ne le déclare pas correctement : une image
// importée reste une image, donc insérée comme image dans la note et non
// comme un simple lien (ADR-006).
func TestTypePieceJointeDeduit(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d}
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	gif := []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;")
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"></svg>`)

	cases := []struct {
		label    string
		filename string
		mime     string
		data     []byte
		want     string
	}{
		{"png sans type déclaré", "capture.png", "", png, "image/png"},
		{"png en type générique", "capture.png", "application/octet-stream", png, "image/png"},
		{"jpeg déduit des octets", "photo.bin", "", jpeg, "image/jpeg"},
		{"gif déduit des octets", "anim", "", gif, "image/gif"},
		{"svg déduit de l'extension", "logo.svg", "", svg, "image/svg+xml"},
		{"type d'image déclaré conservé", "x.png", "image/png", []byte("pas une image"), "image/png"},
		{"fichier texte inchangé", "notes.txt", "", []byte("du texte"), "application/octet-stream"},
		{"type déclaré non-image conservé", "doc.pdf", "application/pdf", []byte("%PDF-1.4"), "application/pdf"},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			svc := newService(t)
			n := addNote(t, svc, "types")
			a, err := svc.AddAttachment(context.Background(), n.ID, notes.AttachmentUpload{
				Filename:   tc.filename,
				Mime:       tc.mime,
				DataBase64: base64.StdEncoding.EncodeToString(tc.data),
			})
			if err != nil {
				t.Fatalf("ajout pièce jointe : %v", err)
			}
			if a.Mime != tc.want {
				t.Errorf("type déduit = %q, attendu %q", a.Mime, tc.want)
			}
		})
	}
}

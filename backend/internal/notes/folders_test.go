package notes_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"notecore/backend/internal/notes"
	"notecore/backend/internal/storage"
)

func mkFolder(t *testing.T, svc *notes.Service, name, parent string) notes.Folder {
	t.Helper()
	f, err := svc.CreateFolderIn(context.Background(), name, parent)
	if err != nil {
		t.Fatalf("création de %q : %v", name, err)
	}
	return f
}

func TestDossiersImbriquesEtNomUniqueParParent(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	s3 := mkFolder(t, svc, "Semestre 3", "")
	s4 := mkFolder(t, svc, "Semestre 4", "")

	// Deux matières peuvent chacune avoir un « TD » ; pas deux dans la même.
	td3 := mkFolder(t, svc, "TD", s3.ID)
	td4 := mkFolder(t, svc, "td", s4.ID)
	if td3.ID == td4.ID || td3.ParentID == nil || *td3.ParentID != s3.ID {
		t.Fatalf("parents mal enregistrés : %+v %+v", td3, td4)
	}
	if _, err := svc.CreateFolderIn(ctx, "Td", s3.ID); err == nil || !strings.Contains(err.Error(), "existe déjà") {
		t.Errorf("doublon sous le même parent accepté : %v", err)
	}
	if _, err := svc.CreateFolder(ctx, "semestre 3"); err == nil {
		t.Error("doublon à la racine accepté (casse ignorée)")
	}
	if _, err := svc.CreateFolderIn(ctx, "X", "inconnu"); err == nil {
		t.Error("parent inconnu accepté")
	}
}

func TestDeplacementDeDossierRefuseLesBoucles(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	a := mkFolder(t, svc, "A", "")
	b := mkFolder(t, svc, "B", a.ID)
	c := mkFolder(t, svc, "C", b.ID)

	for name, move := range map[string][2]string{
		"dans lui-même":         {a.ID, a.ID},
		"dans son enfant":       {a.ID, b.ID},
		"dans son petit-enfant": {a.ID, c.ID},
	} {
		if err := svc.MoveFolder(ctx, move[0], move[1]); err == nil {
			t.Errorf("%s : déplacement accepté", name)
		}
	}
	// Un déplacement valide emporte tout le sous-arbre.
	other := mkFolder(t, svc, "Autre", "")
	if err := svc.MoveFolder(ctx, b.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	folders, _ := svc.ListFolders(ctx)
	for _, f := range folders {
		if f.ID == b.ID && (f.ParentID == nil || *f.ParentID != other.ID) {
			t.Errorf("B mal déplacé : %+v", f)
		}
		if f.ID == c.ID && (f.ParentID == nil || *f.ParentID != b.ID) {
			t.Errorf("C doit suivre B : %+v", f)
		}
	}
	// Vers la racine, puis conflit de nom à destination.
	if err := svc.MoveFolder(ctx, b.ID, ""); err != nil {
		t.Fatal(err)
	}
	mkFolder(t, svc, "B", other.ID)
	if err := svc.MoveFolder(ctx, b.ID, other.ID); err == nil || !strings.Contains(err.Error(), "existe déjà") {
		t.Errorf("conflit de nom à destination : %v", err)
	}
}

func TestProfondeurMaximale(t *testing.T) {
	svc := newService(t)
	parent := ""
	for i := 0; i < notes.MaxFolderDepth; i++ {
		parent = mkFolder(t, svc, string(rune('a'+i)), parent).ID
	}
	if _, err := svc.CreateFolderIn(context.Background(), "trop-profond", parent); err == nil {
		t.Error("la profondeur maximale doit être refusée")
	}
}

func TestSuppressionRemonteNotesEtSousDossiers(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	matiere := mkFolder(t, svc, "Réseaux", "")
	td := mkFolder(t, svc, "TD", matiere.ID)
	chap := mkFolder(t, svc, "Chapitre 1", td.ID)

	n, _ := svc.CreateNote(ctx, "Exercice", td.ID)
	moved, err := svc.DeleteFolder(ctx, td.ID)
	if err != nil {
		t.Fatal(err)
	}
	// La note monte chez le parent, le sous-dossier aussi : rien n'est perdu.
	if len(moved) != 1 || moved[0].ID != n.ID || moved[0].FolderID == nil || *moved[0].FolderID != matiere.ID {
		t.Errorf("notes déplacées = %+v", moved)
	}
	got, _ := svc.GetNote(ctx, n.ID)
	if got.FolderID == nil || *got.FolderID != matiere.ID {
		t.Errorf("la note doit être chez « Réseaux » : %+v", got.FolderID)
	}
	folders, _ := svc.ListFolders(ctx)
	found := false
	for _, f := range folders {
		if f.ID == td.ID {
			t.Error("le dossier supprimé existe encore")
		}
		if f.ID == chap.ID {
			found = true
			if f.ParentID == nil || *f.ParentID != matiere.ID {
				t.Errorf("« Chapitre 1 » doit remonter chez « Réseaux » : %+v", f.ParentID)
			}
		}
	}
	if !found {
		t.Error("le sous-dossier a disparu")
	}

	// Un conflit de nom est signalé, rien n'est supprimé.
	x := mkFolder(t, svc, "X", matiere.ID)
	mkFolder(t, svc, "Chapitre 1", x.ID)
	if _, err := svc.DeleteFolder(ctx, x.ID); err == nil {
		t.Error("remonter un sous-dossier de même nom doit être refusé")
	}
	if f2, _ := svc.ListFolders(ctx); len(f2) != 4 {
		t.Errorf("rien ne doit être supprimé : %d dossiers", len(f2))
	}
}

func TestDeplacementGroupeToutOuRien(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	f := mkFolder(t, svc, "Cours", "")
	a, _ := svc.CreateNote(ctx, "A", "")
	b, _ := svc.CreateNote(ctx, "B", "")

	if err := svc.MoveNotes(ctx, []string{a.ID, b.ID}, f.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{a.ID, b.ID} {
		if n, _ := svc.GetNote(ctx, id); n.FolderID == nil || *n.FolderID != f.ID {
			t.Errorf("note %s non rangée", id)
		}
	}
	// Une note inconnue annule tout le lot.
	if err := svc.MoveNotes(ctx, []string{a.ID, "inconnue"}, ""); err == nil {
		t.Error("une note inconnue doit faire échouer le lot")
	}
	if n, _ := svc.GetNote(ctx, a.ID); n.FolderID == nil {
		t.Error("le lot refusé ne doit rien déplacer")
	}
	if err := svc.MoveNotes(ctx, []string{a.ID}, "inconnu"); err == nil {
		t.Error("un dossier inconnu doit être refusé")
	}
	// Retour à la racine.
	if err := svc.MoveNotes(ctx, []string{a.ID, b.ID}, ""); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.GetNote(ctx, b.ID); n.FolderID != nil {
		t.Error("la note doit être revenue à la racine")
	}
}

func TestRangerNeModifiePasLaDate(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	f := mkFolder(t, svc, "Cours", "")
	n, _ := svc.CreateNote(ctx, "A", "")
	before, _ := svc.GetNote(ctx, n.ID)
	if err := svc.MoveNotes(ctx, []string{n.ID}, f.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := svc.GetNote(ctx, n.ID)
	if after.UpdatedAt != before.UpdatedAt {
		t.Errorf("ranger ne doit pas changer la date de modification : %s → %s", before.UpdatedAt, after.UpdatedAt)
	}
}

// Des données corrompues (une boucle d'ancêtres) ne font ni planter ni tourner
// à vide : l'opération est refusée.
func TestArborescenceCorrompueNePlantePas(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := notes.New(db)
	ctx := context.Background()
	a := mkFolder(t, svc, "A", "")
	b := mkFolder(t, svc, "B", a.ID)
	c := mkFolder(t, svc, "C", "")
	// Boucle A → B → A, écrite directement (l'application, elle, l'interdit).
	if _, err := db.Exec(`UPDATE folders SET parent_id = ? WHERE id = ?`, b.ID, a.ID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- svc.MoveFolder(ctx, a.ID, c.ID) }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("déplacer un dossier pris dans une boucle doit être refusé")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MoveFolder tourne à vide sur une boucle")
	}
	if _, err := svc.SuggestFolders(ctx, mustNote(t, svc, c.ID).ID, 3); err != nil {
		t.Errorf("les propositions doivent survivre à une boucle : %v", err)
	}
	if _, err := svc.Inbox(ctx); err != nil {
		t.Errorf("la boîte à classer doit survivre à une boucle : %v", err)
	}
}

func mustNote(t *testing.T, svc *notes.Service, folderID string) notes.Note {
	t.Helper()
	n, err := svc.CreateNote(context.Background(), "x", folderID)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func tree(name string, children ...notes.FolderSpec) notes.FolderSpec {
	return notes.FolderSpec{Name: name, Children: children}
}

func TestModeleDeMatiere(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	spec := tree("Réseaux", tree("Cours"), tree("TD"), tree("TP"), tree("Révisions"))

	res, err := svc.CreateFolderTree(ctx, "", []notes.FolderSpec{spec})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 5 || res.Existing != 0 || len(res.RootIDs) != 1 {
		t.Fatalf("résultat = %+v", res)
	}
	folders, _ := svc.ListFolders(ctx)
	children := 0
	for _, f := range folders {
		if f.ParentID != nil && *f.ParentID == res.RootIDs[0] {
			children++
		}
	}
	if len(folders) != 5 || children != 4 {
		t.Errorf("structure = %d dossiers, %d enfants", len(folders), children)
	}

	// Le réappliquer complète sans dupliquer (casse ignorée), même avec un dossier de plus.
	again, err := svc.CreateFolderTree(ctx, "", []notes.FolderSpec{tree("réseaux", tree("cours"), tree("Examens"))})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Created) != 1 || again.Existing != 2 || again.RootIDs[0] != res.RootIDs[0] {
		t.Errorf("fusion = %+v", again)
	}
	if f2, _ := svc.ListFolders(ctx); len(f2) != 6 {
		t.Errorf("6 dossiers attendus, %d", len(f2))
	}

	// Sous un parent existant.
	sem := mkFolder(t, svc, "Semestre 3", "")
	in, err := svc.CreateFolderTree(ctx, sem.ID, []notes.FolderSpec{tree("Algo", tree("Cours"))})
	if err != nil || len(in.Created) != 2 {
		t.Fatalf("sous un parent : %+v %v", in, err)
	}
}

func TestModeleToutOuRien(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	// Un nom vide au fond de la structure annule tout.
	bad := tree("Matière", tree("Cours"), tree("TD", tree("  ")))
	if _, err := svc.CreateFolderTree(ctx, "", []notes.FolderSpec{bad}); err == nil {
		t.Fatal("un nom vide doit être refusé")
	}
	if f, _ := svc.ListFolders(ctx); len(f) != 0 {
		t.Errorf("rien ne doit être créé : %d dossiers", len(f))
	}
	if _, err := svc.CreateFolderTree(ctx, "", nil); err == nil {
		t.Error("un modèle vide doit être refusé")
	}
	if _, err := svc.CreateFolderTree(ctx, "inconnu", []notes.FolderSpec{tree("X")}); err == nil {
		t.Error("un parent inconnu doit être refusé")
	}
	if _, err := svc.CreateFolderTree(ctx, "", []notes.FolderSpec{tree(strings.Repeat("x", 101))}); err == nil {
		t.Error("un nom trop long doit être refusé")
	}
}

func TestModeleBornes(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	// Trop profond : refusé, y compris en comptant la profondeur du parent.
	deep := tree("n8")
	for i := 7; i >= 1; i-- {
		deep = tree(string(rune('a'+i)), deep)
	}
	if _, err := svc.CreateFolderTree(ctx, "", []notes.FolderSpec{deep}); err != nil {
		t.Fatalf("8 niveaux doivent passer : %v", err)
	}
	p := mkFolder(t, svc, "P", "")
	if _, err := svc.CreateFolderTree(ctx, p.ID, []notes.FolderSpec{deep}); err == nil {
		t.Error("8 niveaux sous un parent dépassent la profondeur maximale")
	}

	// Trop de dossiers.
	var many []notes.FolderSpec
	for i := 0; i < 201; i++ {
		many = append(many, tree(fmt.Sprintf("d%d", i)))
	}
	if _, err := svc.CreateFolderTree(ctx, "", many); err == nil {
		t.Error("201 dossiers doivent être refusés")
	}
}

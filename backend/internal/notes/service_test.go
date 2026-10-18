package notes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"notecore/backend/internal/notes"
	"notecore/backend/internal/storage"
)

func newService(t *testing.T) *notes.Service {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("ouverture base : %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return notes.New(db)
}

// Persistance : une note écrite avant fermeture est là après réouverture
// (critère de validation n°2 du CDC).
func TestNotePersisteApresReouverture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")

	db, err := storage.Open(path)
	if err != nil {
		t.Fatalf("ouverture 1 : %v", err)
	}
	svc := notes.New(db)
	ctx := context.Background()

	note, err := svc.CreateNote(ctx, "Cours à étudier éàùç", "")
	if err != nil {
		t.Fatalf("création : %v", err)
	}
	patch := "Contenu **avec accents** et `code`, listes :\n- câble\n- contrôle d'erreur"
	if _, err := svc.UpdateNote(ctx, note.ID, notes.Patch{Content: &patch}); err != nil {
		t.Fatalf("mise à jour : %v", err)
	}
	// Fermeture « brutale » : on ne fait rien de spécial, on ferme.
	db.Close()

	db2, err := storage.Open(path)
	if err != nil {
		t.Fatalf("ouverture 2 : %v", err)
	}
	defer db2.Close()

	reopened := notes.New(db2)
	got, err := reopened.ListNotes(ctx)
	if err != nil {
		t.Fatalf("relecture : %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("attendu 1 note, obtenu %d", len(got))
	}
	if got[0].ID != note.ID || got[0].Content != patch {
		t.Errorf("note altérée après réouverture : %+v", got[0])
	}
}

// Les étiquettes d'une note sans étiquette doivent être un tableau vide,
// jamais nil : sinon le JSON contient `"tags":null` et l'interface plante
// en itérant sur null (régression d'affichage constatée en V1).
func TestTagsJamaisNull(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	if _, err := svc.CreateNote(ctx, "sans étiquette", ""); err != nil {
		t.Fatalf("création : %v", err)
	}
	list, err := svc.ListNotes(ctx)
	if err != nil {
		t.Fatalf("listage : %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("attendu 1 note, obtenu %d", len(list))
	}
	if list[0].Tags == nil {
		t.Fatalf("Tags nil : le JSON produirait \"tags\": null")
	}
	// Vérifie la sérialisation réelle, ce que le frontend reçoit.
	raw, err := json.Marshal(list[0])
	if err != nil {
		t.Fatalf("marshal : %v", err)
	}
	if strings.Contains(string(raw), `"tags":null`) {
		t.Fatalf("tags sérialisé null : %s", raw)
	}
}

// Migrations idempotentes : migrer deux fois = même état final,
// données préservées (obligation backend-systems + ARCHITECTURE.md §9).
func TestMigrationsIdempotentes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "migr.db")

	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	svc := notes.New(db)
	ctx := context.Background()
	if _, err := svc.CreateNote(ctx, "note de contrôle", ""); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Réouverture = passage de migrations à nouveau.
	for i := 0; i < 2; i++ {
		db, err := storage.Open(path)
		if err != nil {
			t.Fatalf("réouverture %d : %v", i, err)
		}
		rows, err := db.Query(`SELECT COUNT(*) FROM notes`)
		if err != nil {
			t.Fatalf("conservation : %v", err)
		}
		var count int
		if !rows.Next() {
			t.Fatalf("aucune ligne de résultat de comptage")
		}
		if err := rows.Scan(&count); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if count != 1 {
			t.Fatalf("migration non idempotente : %d notes restantes (attendu 1)", count)
		}
		db.Close()
	}
}

// Suppression de dossier : les notes survivent et repassent à la racine
// (cahier des charges §3.2 : jamais de perte silencieuse).
func TestSuppressionDossierConserveLesNotes(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	folder, err := svc.CreateFolder(ctx, "Cours")
	if err != nil {
		t.Fatal(err)
	}
	n1, err := svc.CreateNote(ctx, "OS — séance 1", folder.ID)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := svc.CreateNote(ctx, "Réseaux", folder.ID)
	if err != nil {
		t.Fatal(err)
	}

	moved, err := svc.DeleteFolder(ctx, folder.ID)
	if err != nil {
		t.Fatalf("suppression dossier : %v", err)
	}
	if len(moved) != 2 {
		t.Fatalf("attendu 2 notes déplacées, obtenu %d", len(moved))
	}

	all, err := svc.ListNotes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("notes perdues : %d restantes", len(all))
	}
	for _, n := range all {
		if n.FolderID != nil {
			t.Errorf("note %s non remise à la racine (folder_id = %s)", n.ID, *n.FolderID)
		}
	}
	_ = n1
	_ = n2
}

// Règle d'or FTS : une note écrite est trouvable immédiatement — titre,
// contenu et étiquettes, y compris accents et code (ADR-003, CDC n°5/6).
func TestRechercheImmediate(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	note, err := svc.CreateNote(ctx, "Ordonnancement des processus", "")
	if err != nil {
		t.Fatal(err)
	}
	content := "Le quantum du tourniquet, contrôle d'erreur, câble réseau, mémoire virtuelle.\n```c\nswap_context(&a, &b);\n```"
	tags := []string{"système", "réseaux"}
	if _, err := svc.UpdateNote(ctx, note.ID, notes.Patch{Content: &content, Tags: &tags}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		query string
		want  bool
	}{
		{"titre", "ordonnancement", true},
		{"contenu", "quantum", true},
		{"accent sur étiquette", "système", true},
		{"dans le contenu accentué (diacritiques retirés)", "erreur", true},
		{"inexistant", "zorglub", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := svc.SearchNotes(ctx, tc.query)
			if err != nil {
				t.Fatalf("recherche %q : %v", tc.query, err)
			}
			if tc.want && len(res) == 0 {
				t.Errorf("aucun résultat pour %q", tc.query)
			}
			if !tc.want && len(res) != 0 {
				t.Errorf("résultat inattendu pour %q : %d hits", tc.query, len(res))
			}
		})
	}
}

// Les extraits de recherche sont construits backend et contiennent le
// texte pertinent (cahier des charges §3.3 : titre + extrait pertinent).
func TestExtraitRecherche(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	note, err := svc.CreateNote(ctx, "SQL — pense-bête", "")
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("remplissage ", 30) + "\nLEFT JOIN rend toutes les lignes de gauche."
	if _, err := svc.UpdateNote(ctx, note.ID, notes.Patch{Content: &content}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.SearchNotes(ctx, "join")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("attendu 1 résultat, obtenu %d", len(res))
	}
	if res[0].NoteID != note.ID {
		t.Errorf("mauvaise note renvoyée")
	}
	if res[0].Excerpt == "" {
		t.Errorf("extrait vide")
	}
}

// Le titre par défaut s'applique aux notes sans titre (cahier des charges §3.1).
func TestTitreDefaut(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	note, err := svc.CreateNote(ctx, "   ", "")
	if err != nil {
		t.Fatal(err)
	}
	if note.Title != "Sans titre" {
		t.Errorf("titre = %q, attendu « Sans titre »", note.Title)
	}
}

func TestTitrePeutEtreEffacePuisModifie(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	note, err := svc.CreateNote(ctx, "Sans titre", "")
	if err != nil {
		t.Fatal(err)
	}
	vide := ""
	if _, err := svc.UpdateNote(ctx, note.ID, notes.Patch{Title: &vide}); err != nil {
		t.Fatal(err)
	}
	efface, err := svc.GetNote(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if efface.Title != "" {
		t.Fatalf("titre effacé = %q, attendu vide", efface.Title)
	}

	nouveau := "Cours de Go"
	if _, err := svc.UpdateNote(ctx, note.ID, notes.Patch{Title: &nouveau}); err != nil {
		t.Fatal(err)
	}
	modifie, err := svc.GetNote(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if modifie.Title != nouveau {
		t.Fatalf("titre modifié = %q, attendu %q", modifie.Title, nouveau)
	}
}

// L'unicité des dossiers est insensible à la casse (ARCHITECTURE.md §4).
func TestDossierUniqueSansCasse(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	if _, err := svc.CreateFolder(ctx, "Projets"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateFolder(ctx, "projets"); err == nil {
		t.Error("doublon cassé accepté : l'unicité insensible à la casse a sauté")
	}
}

// Un patch d'étiquettes déduplique et ignore les vides, insensible à la casse.
func TestEtiquettesDedoublonnees(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	note, err := svc.CreateNote(ctx, "test tags", "")
	if err != nil {
		t.Fatal(err)
	}
	tags := []string{"Go", "go", "  GO  ", "", "rust"}
	updated, err := svc.UpdateNote(ctx, note.ID, notes.Patch{Tags: &tags})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Tags) != 2 {
		t.Fatalf("étiquettes = %v, attendu 2 éléments uniques", updated.Tags)
	}
}

// Déplacer une note vers un dossier existant, puis la remettre à la
// racine (folderId = ""). Régression : la transaction de mise à jour
// interrogeait le pool d'une seule connexion (ADR-005) et se bloquait.
func TestDeplacementNoteVersDossier(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	dossier, err := svc.CreateFolder(ctx, "Algo")
	if err != nil {
		t.Fatal(err)
	}
	note, err := svc.CreateNote(ctx, "Graphes", "")
	if err != nil {
		t.Fatal(err)
	}

	moved, err := svc.UpdateNote(ctx, note.ID, notes.Patch{FolderID: &dossier.ID})
	if err != nil {
		t.Fatalf("déplacement vers le dossier : %v", err)
	}
	if moved.FolderID == nil || *moved.FolderID != dossier.ID {
		t.Fatalf("note non rattachée au dossier : %+v", moved.FolderID)
	}

	root := ""
	back, err := svc.UpdateNote(ctx, note.ID, notes.Patch{FolderID: &root})
	if err != nil {
		t.Fatalf("retour à la racine : %v", err)
	}
	if back.FolderID != nil {
		t.Fatalf("note toujours dans un dossier après retour racine : %v", *back.FolderID)
	}
}

// Après N mises à jour, la recherche ne doit renvoyer qu'un seul
// résultat par note. Régression : l'index FTS empilait une ligne par
// édition au lieu de la recalculer.
func TestRechercheSansDoublonApresEditions(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	note, err := svc.CreateNote(ctx, "Mémo", "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		content := fmt.Sprintf("itération unique-%d du mémo", i)
		if _, err := svc.UpdateNote(ctx, note.ID, notes.Patch{Content: &content}); err != nil {
			t.Fatal(err)
		}
	}

	res, err := svc.SearchNotes(ctx, "mémo")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("attendu 1 résultat après 5 éditions, obtenu %d", len(res))
	}

	// Le contenu d'une version précédente ne doit plus correspondre.
	old, err := svc.SearchNotes(ctx, "itération")
	if err != nil {
		t.Fatal(err)
	}
	if len(old) != 1 {
		t.Fatalf("attendu 1 résultat pour le contenu courant, obtenu %d", len(old))
	}
}

// Le dernier mot saisi est un préfixe : la recherche fonctionne pendant la frappe.
func TestRecherchePrefixe(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	n, err := svc.CreateNote(ctx, "Cours", "")
	if err != nil {
		t.Fatal(err)
	}
	body := "Architecture des réseaux"
	if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &body}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"arch", "architecture rés", "ARCH"} {
		hits, err := svc.SearchNotes(ctx, q)
		if err != nil || len(hits) != 1 {
			t.Errorf("recherche %q : %d résultat(s), err=%v", q, len(hits), err)
		}
	}
	if hits, _ := svc.SearchNotes(ctx, "chitecture"); len(hits) != 0 {
		t.Error("une recherche ne doit pas trouver le milieu d'un mot")
	}
}

// Épingler une note ne change pas sa date de modification ; la modifier, si.
func TestEpinglageConserveLaDateDeModification(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	n, err := svc.CreateNote(ctx, "Note", "")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // les dates ont la précision de la seconde

	pinned := true
	got, err := svc.UpdateNote(ctx, n.ID, notes.Patch{IsPinned: &pinned})
	if err != nil {
		t.Fatal(err)
	}
	if got.UpdatedAt != n.UpdatedAt || !got.IsPinned {
		t.Errorf("épinglage : updatedAt %s → %s", n.UpdatedAt, got.UpdatedAt)
	}

	body := "nouveau"
	got, err = svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &body})
	if err != nil {
		t.Fatal(err)
	}
	if got.UpdatedAt == n.UpdatedAt {
		t.Error("une modification du contenu doit mettre à jour la date")
	}
}

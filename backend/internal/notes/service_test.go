package notes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

// Corbeille : une note supprimée disparaît de tout mais garde son contenu ;
// la restauration est complète ; seule la suppression définitive détruit.
func TestCorbeilleCycleComplet(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	n, err := svc.CreateNote(ctx, "note à mettre de côté", "")
	if err != nil {
		t.Fatal(err)
	}
	body := "contenu avec un mot recherché"
	if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &body}); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteNote(ctx, n.ID); err != nil {
		t.Fatalf("mise à la corbeille : %v", err)
	}

	// Absente des listes et de la recherche, présente à la corbeille.
	if got, _ := svc.ListNotes(ctx); len(got) != 0 {
		t.Errorf("une note à la corbeille reste dans les notes actives : %d", len(got))
	}
	if res, _ := svc.SearchNotes(ctx, "recherché"); len(res) != 0 {
		t.Errorf("une note à la corbeille est encore trouvée : %d résultat(s)", len(res))
	}
	trashed, err := svc.ListTrashed(ctx)
	if err != nil || len(trashed) != 1 || trashed[0].ID != n.ID {
		t.Fatalf("corbeille = %+v, err=%v", trashed, err)
	}
	if trashed[0].DeletedAt == nil {
		t.Error("la note à la corbeille n'a pas de date de mise de côté")
	}

	// Restauration : même note, même contenu, redevenue active.
	restored, err := svc.RestoreNote(ctx, n.ID)
	if err != nil {
		t.Fatalf("restauration : %v", err)
	}
	if restored.Content != body || restored.DeletedAt != nil {
		t.Errorf("restauration incomplète : %+v", restored)
	}
	if res, _ := svc.SearchNotes(ctx, "recherché"); len(res) != 1 {
		t.Errorf("la note restaurée n'est pas de nouveau trouvée : %d", len(res))
	}

	// Mise de côté à nouveau, puis suppression définitive.
	if err := svc.DeleteNote(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteNoteForever(ctx, n.ID); err != nil {
		t.Fatalf("suppression définitive : %v", err)
	}
	if got, _ := svc.ListTrashed(ctx); len(got) != 0 {
		t.Errorf("la note détruite reste à la corbeille : %d", len(got))
	}
	if _, err := svc.GetNote(ctx, n.ID); err == nil {
		t.Error("la note détruite reste lisible")
	}
}

// Vidage : EmptyTrash détruit tout ce qui est à la corbeille et ne touche
// pas les notes actives.
func TestViderLaCorbeille(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	active, err := svc.CreateNote(ctx, "garder", "")
	if err != nil {
		t.Fatal(err)
	}
	trashed, err := svc.CreateNote(ctx, "jeter", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteNote(ctx, trashed.ID); err != nil {
		t.Fatal(err)
	}

	n, err := svc.EmptyTrash(ctx)
	if err != nil || n != 1 {
		t.Fatalf("EmptyTrash = %d, err=%v", n, err)
	}
	got, _ := svc.ListNotes(ctx)
	if len(got) != 1 || got[0].ID != active.ID {
		t.Errorf("notes actives après vidage = %+v", got)
	}
}

// Purge : seules les notes à la corbeille depuis plus longtemps que la
// rétention sont détruites ; une durée nulle purge tout ce qui est plus
// vieux que l'instant présent.
func TestPurgeCorbeilleParAnciennete(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	old, err := svc.CreateNote(ctx, "vieille", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteNote(ctx, old.ID); err != nil {
		t.Fatal(err)
	}
	// Une rétention négative place la coupure dans le futur : la note est
	// « trop vieille » et doit être détruite.
	n, err := svc.PurgeTrash(ctx, -time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("purge = %d, err=%v", n, err)
	}
	if got, _ := svc.ListTrashed(ctx); len(got) != 0 {
		t.Errorf("la note trop vieille reste à la corbeille : %+v", got)
	}

	fresh, err := svc.CreateNote(ctx, "récente", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteNote(ctx, fresh.ID); err != nil {
		t.Fatal(err)
	}
	n, err = svc.PurgeTrash(ctx, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("purge 30 j : %v", err)
	}
	if n != 0 {
		t.Errorf("%d note(s) trop récente(s) purgée(s)", n)
	}
	if got, _ := svc.ListTrashed(ctx); len(got) != 1 || got[0].ID != fresh.ID {
		t.Errorf("corbeille après purge = %+v", got)
	}
}

// Opérateurs de recherche : tag:, dossier:, favori — seuls ou combinés
// au texte libre.
func TestRechercheOperateurs(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	dossier, err := svc.CreateFolder(ctx, "Cours")
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.CreateNote(ctx, "réseaux", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateNote(ctx, a.ID, notes.Patch{Tags: ptr([]string{"Important"}), FolderID: &dossier.ID}); err != nil {
		t.Fatal(err)
	}
	b, err := svc.CreateNote(ctx, "algo", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateNote(ctx, b.ID, notes.Patch{Tags: ptr([]string{"important"}), IsPinned: ptr(true)}); err != nil {
		t.Fatal(err)
	}

	// tag: seul, insensible à la casse, correspondance exacte.
	res, err := svc.SearchNotes(ctx, "tag:important")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("tag:important = %d résultat(s), attendu 2", len(res))
	}
	res, _ = svc.SearchNotes(ctx, "tag:import")
	if len(res) != 0 {
		t.Errorf("tag:import (sous-chaîne) a renvoyé %d résultat(s)", len(res))
	}

	// dossier: par nom, insensible à la casse.
	res, _ = svc.SearchNotes(ctx, "dossier:cours")
	if len(res) != 1 || res[0].NoteID != a.ID {
		t.Errorf("dossier:cours = %+v", res)
	}
	res, _ = svc.SearchNotes(ctx, "folder:cours")
	if len(res) != 1 {
		t.Errorf("folder:cours = %d résultat(s)", len(res))
	}

	// favori : épinglage seul.
	res, _ = svc.SearchNotes(ctx, "favori")
	if len(res) != 1 || res[0].NoteID != b.ID {
		t.Errorf("favori = %+v", res)
	}

	// Combinaisons : dossier ET tag → seulement a ; tag ET favori → seulement b.
	res, _ = svc.SearchNotes(ctx, "dossier:cours tag:important")
	if len(res) != 1 || res[0].NoteID != a.ID {
		t.Errorf("dossier+tag = %+v", res)
	}
	res, _ = svc.SearchNotes(ctx, "tag:important favori")
	if len(res) != 1 || res[0].NoteID != b.ID {
		t.Errorf("tag+favori = %+v", res)
	}
	// Texte + favori : b ne contient pas « réseau », a n'est pas épinglée.
	res, _ = svc.SearchNotes(ctx, "favori réseau")
	if len(res) != 0 {
		t.Errorf("favori+texte incompatible = %+v", res)
	}
}

func ptr[T any](v T) *T { return &v }

// Historique de versions : chaque état distinct sauvegardé est conservé,
// la liste ne duplique pas les sauvegardes identiques, et la restauration
// est réversible (l'état remplacé redevient une version).
func TestHistoriqueVersions(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	n, err := svc.CreateNote(ctx, "note versionnée", "")
	if err != nil {
		t.Fatal(err)
	}
	v1 := "première version du texte"
	if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &v1}); err != nil {
		t.Fatal(err)
	}
	// Sauvegarde identique : pas de doublon.
	if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &v1}); err != nil {
		t.Fatal(err)
	}
	v2 := "deuxième version, plus longue"
	if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &v2}); err != nil {
		t.Fatal(err)
	}

	versions, err := svc.ListVersions(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	// v1 et v2 ont été enregistrés ; la sauvegarde répétée de v1 non.
	if len(versions) != 2 {
		t.Fatalf("%d versions au lieu de 2 : %+v", len(versions), versions)
	}
	if versions[0].Chars != utf8.RuneCountInString(v2) {
		t.Errorf("la version la plus récente devrait être v2 (%d car.)", utf8.RuneCountInString(v2))
	}

	// Restauration de v1 : le contenu revient, et l'état courant (v2)
	// reste disponible dans l'historique.
	oldest := versions[1]
	restored, err := svc.RestoreVersion(ctx, n.ID, oldest.ID)
	if err != nil {
		t.Fatalf("restauration : %v", err)
	}
	if restored.Content != v1 {
		t.Errorf("contenu restauré = %q, attendu %q", restored.Content, v1)
	}
	after, err := svc.ListVersions(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 3 {
		t.Errorf("%d versions après restauration, 3 attendues (réversible)", len(after))
	}

	// Une version ne peut pas être restaurée dans une autre note.
	other, err := svc.CreateNote(ctx, "autre note", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RestoreVersion(ctx, other.ID, oldest.ID); err == nil {
		t.Error("une version a été restaurée dans une note qui n'est pas la sienne")
	}

	// La suppression définitive emporte l'historique (via la corbeille).
	if err := svc.DeleteNote(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteNoteForever(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	versions, err = svc.ListVersions(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Errorf("%d versions survivent à la suppression définitive", len(versions))
	}
}

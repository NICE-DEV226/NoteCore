package notes_test

import (
	"context"
	"testing"

	"notecore/backend/internal/notes"
)

// corpus range quelques notes de trois matières ; renvoie les dossiers.
func corpus(t *testing.T, svc *notes.Service) (reseaux, algo, bdd notes.Folder) {
	t.Helper()
	ctx := context.Background()
	reseaux = mkFolder(t, svc, "Réseaux", "")
	algo = mkFolder(t, svc, "Algorithmique", "")
	bdd = mkFolder(t, svc, "Bases de données", "")
	add := func(f notes.Folder, title, body string, tags ...string) notes.Note {
		n, err := svc.CreateNote(ctx, title, f.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &body, Tags: &tags}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	add(reseaux, "Modèle OSI", "Les sept couches du modèle OSI : physique, liaison, réseau, transport, session. Le protocole TCP fiabilise le transport, UDP privilégie la rapidité. Adresse IP et routage entre réseaux.", "réseaux")
	add(reseaux, "TCP et UDP", "Poignée de main en trois temps du protocole TCP, ports, segments, contrôle de congestion. UDP sans connexion pour le streaming.", "réseaux")
	add(reseaux, "Routage IP", "Tables de routage, passerelle, masque de sous-réseau, protocole OSPF et BGP, paquets IP.", "réseaux")
	add(algo, "Tri rapide", "Algorithme de tri rapide : choisir un pivot, partitionner le tableau, récursion. Complexité moyenne n log n, pire cas quadratique.", "algorithmique")
	add(algo, "Graphes", "Parcours en largeur et en profondeur d'un graphe, algorithme de Dijkstra, complexité, file de priorité.", "algorithmique")
	add(bdd, "Jointures SQL", "Requêtes SQL : INNER JOIN, LEFT JOIN, clé étrangère, index, transaction et normalisation des tables.", "sql")
	add(bdd, "Transactions", "Propriétés ACID d'une transaction SQL, verrous, niveaux d'isolation, journal de la base de données.", "sql")
	return
}

func TestSuggestionDeDossier(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	reseaux, algo, _ := corpus(t, svc)

	n, _ := svc.CreateNote(ctx, "Pare-feu et NAT", "")
	body := "Le NAT traduit les adresses IP privées, le pare-feu filtre les paquets selon le protocole et les ports TCP."
	tags := []string{"réseaux"}
	if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &body, Tags: &tags}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.SuggestFolders(ctx, n.ID, 3)
	if err != nil || len(got) == 0 {
		t.Fatalf("suggestions = %+v, err=%v", got, err)
	}
	if got[0].FolderID != reseaux.ID {
		t.Fatalf("« Réseaux » attendu en tête, obtenu %v (%+v)", got[0].Path, got)
	}
	if got[0].Confidence != "high" {
		t.Errorf("confiance = %q, attendu high (score %.2f)", got[0].Confidence, got[0].Score)
	}
	if len(got[0].Tags) == 0 || got[0].Tags[0] != "reseaux" || len(got[0].Terms) == 0 {
		t.Errorf("la proposition doit s'expliquer (étiquettes, mots) : %+v", got[0])
	}
	for _, s := range got[1:] {
		if s.FolderID == algo.ID && s.Score >= got[0].Score {
			t.Error("un mauvais dossier ne doit pas égaler le bon")
		}
	}
}

func TestSuggestionSansEtiquette(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	_, algo, _ := corpus(t, svc)

	n, _ := svc.CreateNote(ctx, "Tri fusion", "")
	body := "Le tri fusion divise le tableau en deux, trie récursivement chaque moitié puis fusionne. Complexité n log n garantie."
	if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &body}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.SuggestFolders(ctx, n.ID, 3)
	if len(got) == 0 || got[0].FolderID != algo.ID {
		t.Fatalf("« Algorithmique » attendu d'après le seul vocabulaire : %+v", got)
	}
}

func TestSuggestionHonnete(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	corpus(t, svc)

	// Hors sujet : aucune proposition plutôt qu'une proposition au hasard.
	n, _ := svc.CreateNote(ctx, "Courses", "")
	body := "Acheter du pain, des pommes et du lait pour la semaine."
	if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &body}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.SuggestFolders(ctx, n.ID, 3); len(got) > 0 && got[0].Confidence == "high" {
		t.Errorf("une note hors sujet ne doit pas recevoir de proposition fiable : %+v", got[0])
	}
	// Trop courte : rien à comparer.
	short, _ := svc.CreateNote(ctx, "TCP", "")
	if got, _ := svc.SuggestFolders(ctx, short.ID, 3); len(got) != 0 {
		t.Errorf("une note vide ne doit rien suggérer : %+v", got)
	}
	if _, err := svc.SuggestFolders(ctx, "inconnue", 3); err == nil {
		t.Error("note inconnue acceptée")
	}
}

func TestSuggestionNeSeReconnaitPasElleMeme(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	reseaux, algo, _ := corpus(t, svc)

	// Une note de réseaux rangée par erreur dans « Algorithmique » : la
	// proposition doit la renvoyer vers « Réseaux », pas la confirmer.
	n, _ := svc.CreateNote(ctx, "Protocole HTTP", algo.ID)
	body := "Le protocole HTTP s'appuie sur TCP et les ports, avec une adresse IP, un routage et des paquets entre réseaux."
	tags := []string{"réseaux"}
	if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &body, Tags: &tags}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.SuggestFolders(ctx, n.ID, 2)
	if len(got) == 0 || got[0].FolderID != reseaux.ID {
		t.Errorf("« Réseaux » attendu : %+v", got)
	}
}

func TestBoiteACLasser(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	reseaux, _, bdd := corpus(t, svc)

	mk := func(title, body string) notes.Note {
		n, _ := svc.CreateNote(ctx, title, "")
		if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &body}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	a := mk("Sous-réseaux", "Découper un réseau IP en sous-réseaux avec un masque, passerelle et routage des paquets.")
	b := mk("Index SQL", "Un index accélère les requêtes SQL sur une table de la base ; clé étrangère et jointure.")
	c := mk("Divers", "xyz")

	inbox, err := svc.Inbox(ctx)
	if err != nil || len(inbox) != 3 {
		t.Fatalf("3 notes à classer attendues : %d, err=%v", len(inbox), err)
	}
	want := map[string]string{a.ID: reseaux.ID, b.ID: bdd.ID}
	for _, it := range inbox {
		if target, ok := want[it.NoteID]; ok {
			if len(it.Suggestions) == 0 || it.Suggestions[0].FolderID != target {
				t.Errorf("proposition de %s = %+v, attendu %s", it.NoteID, it.Suggestions, target)
			}
		}
		if it.NoteID == c.ID && len(it.Suggestions) != 0 {
			t.Errorf("« Divers » ne doit rien suggérer : %+v", it.Suggestions)
		}
	}
	// Les notes sans proposition viennent en dernier.
	if inbox[len(inbox)-1].NoteID != c.ID {
		t.Error("les notes sans proposition doivent venir en dernier")
	}
	// Une note rangée quitte la boîte.
	if err := svc.MoveNotes(ctx, []string{a.ID}, reseaux.ID); err != nil {
		t.Fatal(err)
	}
	if inbox, _ = svc.Inbox(ctx); len(inbox) != 2 {
		t.Errorf("la note rangée doit sortir de la boîte : %d", len(inbox))
	}
}

func TestSuggestionDossierParent(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	matiere := mkFolder(t, svc, "Systèmes", "")
	td := mkFolder(t, svc, "TD", matiere.ID)
	for _, body := range []string{
		"Ordonnancement des processus : quantum, tourniquet, priorité, ordonnanceur du noyau.",
		"Mémoire virtuelle, pagination, défaut de page, tampon de traduction, noyau.",
	} {
		n, _ := svc.CreateNote(ctx, "TD système", td.ID)
		b := body
		if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &b}); err != nil {
			t.Fatal(err)
		}
	}
	n, _ := svc.CreateNote(ctx, "Processus", "")
	b := "Un processus a un état : prêt, en cours, bloqué ; l'ordonnanceur du noyau choisit le suivant."
	if _, err := svc.UpdateNote(ctx, n.ID, notes.Patch{Content: &b}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.SuggestFolders(ctx, n.ID, 3)
	if len(got) < 2 {
		t.Fatalf("« TD » et son parent attendus : %+v", got)
	}
	// Les deux niveaux sont proposés, le dossier le plus précis d'abord.
	if got[0].FolderID != td.ID || got[1].FolderID != matiere.ID {
		t.Errorf("ordre = %v puis %v", got[0].Path, got[1].Path)
	}
	if len(got[0].Path) != 2 || got[0].Path[0] != "Systèmes" || got[0].Path[1] != "TD" {
		t.Errorf("chemin = %v", got[0].Path)
	}
}

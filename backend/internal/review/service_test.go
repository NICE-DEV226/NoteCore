package review_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"notecore/backend/internal/notes"
	"notecore/backend/internal/review"
	"notecore/backend/internal/storage"
)

type env struct {
	db    *sql.DB
	notes *notes.Service
	rev   *review.Service
	ctx   context.Context
}

func setup(t *testing.T) env {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return env{db: db, notes: notes.New(db), rev: review.New(db), ctx: context.Background()}
}

func (e env) write(t *testing.T, title, content string) notes.Note {
	t.Helper()
	n, err := e.notes.CreateNote(e.ctx, title, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.notes.UpdateNote(e.ctx, n.ID, notes.Patch{Content: &content}); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e env) count(t *testing.T, where string) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM cards WHERE ` + where).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Écrire une carte dans une note la crée ; la retirer la met de côté ; la
// réécrire la réactive avec sa progression.
func TestSynchroniseLesCartesAvecLaNote(t *testing.T) {
	e := setup(t)
	n := e.write(t, "Réseaux", "Le {{c1::TCP}} est fiable.\nQuel port pour HTTPS ? :: 443")
	if got := e.count(t, "orphaned_at IS NULL"); got != 2 {
		t.Fatalf("2 cartes attendues, obtenu %d", got)
	}

	empty := "plus rien"
	if _, err := e.notes.UpdateNote(e.ctx, n.ID, notes.Patch{Content: &empty}); err != nil {
		t.Fatal(err)
	}
	if e.count(t, "orphaned_at IS NULL") != 0 || e.count(t, "orphaned_at IS NOT NULL") != 2 {
		t.Fatal("les cartes disparues du texte doivent être mises de côté, pas détruites")
	}

	back := "Le {{c1::TCP}} est fiable."
	if _, err := e.notes.UpdateNote(e.ctx, n.ID, notes.Patch{Content: &back}); err != nil {
		t.Fatal(err)
	}
	if e.count(t, "orphaned_at IS NULL") != 1 {
		t.Error("une carte qui réapparaît doit être réactivée")
	}
}

func TestSupprimerLaNoteEmporteSesCartes(t *testing.T) {
	e := setup(t)
	n := e.write(t, "N", "{{c1::a}} {{c2::b}}")
	if err := e.notes.DeleteNote(e.ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.count(t, "1=1"); got != 0 {
		t.Errorf("%d carte(s) orpheline(s) après suppression de la note", got)
	}
}

func TestRevisionComplete(t *testing.T) {
	e := setup(t)
	e.write(t, "N", "Le {{c1::noyau}} gère les processus.")

	st, err := e.rev.Stats(e.ctx)
	if err != nil || st.New != 1 || st.Total != 1 {
		t.Fatalf("stats initiales = %+v, err=%v", st, err)
	}

	card, err := e.rev.Next(e.ctx)
	if err != nil || card == nil {
		t.Fatalf("une carte attendue : %v %v", card, err)
	}
	if card.Front != "Le […] gère les processus." || card.Back != "Le ==noyau== gère les processus." {
		t.Errorf("recto/verso = %q / %q", card.Front, card.Back)
	}
	if card.Intervals.Good == "" || card.Intervals.Again == "" {
		t.Errorf("les délais proposés doivent être formulés : %+v", card.Intervals)
	}

	if err := e.rev.Answer(e.ctx, card.ID, 3); err != nil {
		t.Fatal(err)
	}
	// Une carte apprise n'est plus « nouvelle » ; elle est planifiée plus tard.
	st, _ = e.rev.Stats(e.ctx)
	if st.New != 0 || st.ReviewedToday != 1 {
		t.Errorf("après réponse : %+v", st)
	}
	var state int
	var due string
	if err := e.db.QueryRow(`SELECT state, due FROM cards`).Scan(&state, &due); err != nil {
		t.Fatal(err)
	}
	if state == 0 {
		t.Error("la carte doit avoir quitté l'état « nouvelle »")
	}
	if due <= time.Now().UTC().Format("2006-01-02T15:04:05.000Z") {
		t.Errorf("la prochaine échéance doit être dans le futur : %s", due)
	}
}

func TestAnnulerLaDerniereReponse(t *testing.T) {
	e := setup(t)
	e.write(t, "N", "{{c1::x}} est ici")
	card, _ := e.rev.Next(e.ctx)
	if err := e.rev.Answer(e.ctx, card.ID, 4); err != nil {
		t.Fatal(err)
	}
	undone, err := e.rev.Undo(e.ctx)
	if err != nil || undone == nil || undone.ID != card.ID {
		t.Fatalf("annulation : %+v %v", undone, err)
	}
	if undone.State != 0 {
		t.Errorf("la carte doit retrouver son état d'avant (nouvelle), état = %d", undone.State)
	}
	if got, _ := e.rev.Undo(e.ctx); got != nil {
		t.Error("plus rien à annuler")
	}
	st, _ := e.rev.Stats(e.ctx)
	if st.ReviewedToday != 0 || st.New != 1 {
		t.Errorf("l'historique doit être retiré aussi : %+v", st)
	}
}

func TestLimiteDeNouvellesCartes(t *testing.T) {
	e := setup(t)
	e.write(t, "N", "{{c1::a}} {{c2::b}} {{c3::c}}")
	if _, err := e.db.Exec(`INSERT INTO settings(key, value) VALUES('review.newPerDay', '2')`); err != nil {
		t.Fatal(err)
	}
	st, _ := e.rev.Stats(e.ctx)
	if st.New != 2 {
		t.Fatalf("2 nouvelles cartes par jour attendues, obtenu %d", st.New)
	}
	for i := 0; i < 2; i++ {
		c, _ := e.rev.Next(e.ctx)
		if err := e.rev.Answer(e.ctx, c.ID, 3); err != nil {
			t.Fatal(err)
		}
	}
	if st, _ = e.rev.Stats(e.ctx); st.New != 0 {
		t.Errorf("limite atteinte : %+v", st)
	}
}

func TestReponseInvalide(t *testing.T) {
	e := setup(t)
	e.write(t, "N", "{{c1::a}}")
	c, _ := e.rev.Next(e.ctx)
	for _, r := range []int{0, 5, -1} {
		if err := e.rev.Answer(e.ctx, c.ID, r); err == nil {
			t.Errorf("la réponse %d doit être refusée", r)
		}
	}
	if err := e.rev.Answer(e.ctx, "inconnue", 3); err == nil {
		t.Error("une carte inconnue doit être refusée")
	}
}

func TestMaitriseParDossierEtEtiquette(t *testing.T) {
	e := setup(t)
	folder, err := e.notes.CreateFolder(e.ctx, "Cours")
	if err != nil {
		t.Fatal(err)
	}
	n := e.write(t, "N", "{{c1::a}} et {{c2::b}}")
	tags := []string{"os"}
	fid := folder.ID
	if _, err := e.notes.UpdateNote(e.ctx, n.ID, notes.Patch{FolderID: &fid, Tags: &tags}); err != nil {
		t.Fatal(err)
	}
	c, _ := e.rev.Next(e.ctx)
	if err := e.rev.Answer(e.ctx, c.ID, 4); err != nil {
		t.Fatal(err)
	}
	rows, err := e.rev.Mastery(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var sawFolder, sawTag bool
	for _, r := range rows {
		if r.Label == "Cours" && r.Kind == "folder" && r.Cards == 2 && r.Learned == 1 && r.Retention > 0.5 {
			sawFolder = true
		}
		if r.Label == "os" && r.Kind == "tag" && r.Cards == 2 {
			sawTag = true
		}
	}
	if !sawFolder || !sawTag {
		t.Errorf("maîtrise incomplète : %+v", rows)
	}
}

func TestIndexationDesNotesExistantes(t *testing.T) {
	e := setup(t)
	// Note écrite directement (comme par une version sans cartes).
	if _, err := e.db.Exec(`INSERT INTO notes (id, title, content, tags_text, created_at, updated_at)
		VALUES ('old', 'Ancienne', 'Q ? :: R', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := e.rev.Maintain(e.ctx); err != nil {
		t.Fatal(err)
	}
	if e.count(t, "note_id = 'old'") != 1 {
		t.Error("les notes existantes doivent être indexées au démarrage")
	}
	if err := e.rev.Maintain(e.ctx); err != nil { // idempotent
		t.Fatal(err)
	}
	if e.count(t, "note_id = 'old'") != 1 {
		t.Error("l'indexation ne doit pas dupliquer les cartes")
	}
}

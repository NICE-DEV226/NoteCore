// Bindings de la révision espacée (ADR-007). Ils restent minces : le moteur
// (FSRS, planification, maîtrise) vit dans le paquet review.
package bindings

import (
	"context"
	"fmt"
	"strconv"

	"notecore/backend/internal/review"
)

// MaintainReview indexe les notes existantes et purge les cartes périmées.
// Appelé une fois au démarrage.
func (n *NoteCore) MaintainReview(ctx context.Context) error {
	return n.rev.Maintain(ctx)
}

// ReviewStats : à réviser, nouvelles cartes, série, activité des 14 jours.
func (n *NoteCore) ReviewStats() (review.Stats, error) {
	return n.rev.Stats(n.callContext())
}

// ReviewNext renvoie la prochaine carte, ou null quand il n'y en a plus.
func (n *NoteCore) ReviewNext() (*review.CardView, error) {
	return n.rev.Next(n.callContext())
}

// ReviewAnswer enregistre une réponse : 1 À revoir, 2 Difficile, 3 Bien, 4 Facile.
func (n *NoteCore) ReviewAnswer(cardID string, rating int) error {
	if cardID == "" {
		return fmt.Errorf("carte non identifiée")
	}
	return n.rev.Answer(n.callContext(), cardID, rating)
}

// ReviewUndo annule la dernière réponse et renvoie la carte concernée.
func (n *NoteCore) ReviewUndo() (*review.CardView, error) {
	return n.rev.Undo(n.callContext())
}

// ReviewMastery : maîtrise par dossier et par étiquette.
func (n *NoteCore) ReviewMastery() ([]review.MasteryRow, error) {
	rows, err := n.rev.Mastery(n.callContext())
	if rows == nil {
		rows = []review.MasteryRow{}
	}
	return rows, err
}

// NoteCards résume les cartes d'une note.
func (n *NoteCore) NoteCards(noteID string) (review.NoteSummary, error) {
	if noteID == "" {
		return review.NoteSummary{}, fmt.Errorf("note non identifiée")
	}
	return n.rev.NoteSummary(n.callContext(), noteID)
}

// ReviewSettings renvoie les réglages courants de la révision.
func (n *NoteCore) ReviewSettings() (review.Settings, error) {
	return n.rev.Settings(n.callContext()), nil
}

// SetReviewSettings enregistre les réglages, bornés : nouvelles cartes par
// jour (0 à 500) et rétention visée (0,70 à 0,99).
func (n *NoteCore) SetReviewSettings(newPerDay int, retention float64) error {
	if newPerDay < 0 || newPerDay > 500 {
		return fmt.Errorf("nouvelles cartes par jour : entre 0 et 500")
	}
	if retention < 0.7 || retention > 0.99 {
		return fmt.Errorf("rétention visée : entre 70 %% et 99 %%")
	}
	ctx := n.callContext()
	if err := n.prefs.Set(ctx, "review.newPerDay", strconv.Itoa(newPerDay)); err != nil {
		return err
	}
	return n.prefs.Set(ctx, "review.retention", strconv.FormatFloat(retention, 'f', 2, 64))
}

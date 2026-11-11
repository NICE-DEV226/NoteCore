package bindings

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gen2brain/beeep"
)

// Notifications OS des révisions dues : une invitation discrète quand des
// cartes attendent, jamais plus d'une par heure. Le réglage
// « reviewNotifications » ("0" = muet) est lu à chaque passage ; l'échec
// d'affichage (pas de daemon de notifications, session minimale) est
// ignoré — la révision reste accessible dans l'application.
const (
	reviewNotifySetting = "reviewNotifications"
	reviewNotifyEvery   = 15 * time.Minute
	reviewNotifyGap     = time.Hour
)

// reviewNotifier tourne jusqu'à la fermeture du contexte Wails.
func (n *NoteCore) reviewNotifier(ctx context.Context) {
	ticker := time.NewTicker(reviewNotifyEvery)
	defer ticker.Stop()
	// Premier passage après une courte attente : laisser l'interface se
	// charger avant de proposer une révision.
	first := time.NewTimer(90 * time.Second)
	defer first.Stop()

	var lastNotify time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-first.C:
			n.maybeNotifyReview(ctx, &lastNotify)
		case <-ticker.C:
			n.maybeNotifyReview(ctx, &lastNotify)
		}
	}
}

func (n *NoteCore) maybeNotifyReview(ctx context.Context, last *time.Time) {
	if v, err := n.prefs.Get(ctx, reviewNotifySetting); err != nil || v == "0" {
		return
	}
	stats, err := n.rev.Stats(ctx)
	if err != nil || stats.Due == 0 {
		return
	}
	if time.Since(*last) < reviewNotifyGap {
		return
	}
	msg := "cartes à réviser"
	if stats.Due == 1 {
		msg = "carte à réviser"
	}
	if err := beeep.Notify("NoteCore", fmt.Sprintf("%d %s", stats.Due, msg), nil); err != nil {
		log.Printf("notification de révision : %v", err)
		return
	}
	*last = time.Now()
}

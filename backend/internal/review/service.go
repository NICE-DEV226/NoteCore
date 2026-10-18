package review

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-spaced-repetition/go-fsrs/v4"
)

// tsFormat : horodatage à largeur fixe, en UTC. Le tri lexicographique de la
// colonne `due` est alors exactement le tri chronologique.
const tsFormat = "2006-01-02T15:04:05.000Z"

const (
	// learnAhead : une carte en apprentissage due dans moins de ce délai est
	// proposée tout de suite plutôt que de laisser la session s'arrêter.
	learnAhead = 20 * time.Minute
	// orphanGrace : une carte dont le texte a disparu de la note est mise de
	// côté, pas détruite — l'utilisateur peut avoir retouché la question, ou
	// annulé une frappe. Elle est purgée passé ce délai.
	orphanGrace = 30 * 24 * time.Hour

	settingNewPerDay = "review.newPerDay"
	settingRetention = "review.retention"
	settingIndexed   = "review.indexed"

	defaultNewPerDay = 20
	defaultRetention = 0.9
)

// Service est le moteur de révision. Il ne connaît que SQLite et FSRS.
type Service struct {
	db  *sql.DB
	now func() time.Time
}

func New(db *sql.DB) *Service {
	return &Service{db: db, now: time.Now}
}

// dbtx est satisfait par *sql.DB comme par *sql.Tx : la synchronisation doit
// pouvoir s'exécuter DANS la transaction de la note (règle d'or, ADR-003).
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Réglages -------------------------------------------------------------------

// Settings sont les réglages de révision, lus dans la table `settings`.
type Settings struct {
	NewPerDay int     `json:"newPerDay"`
	Retention float64 `json:"retention"`
}

func (s *Service) settings(ctx context.Context, q dbtx) Settings {
	out := Settings{NewPerDay: defaultNewPerDay, Retention: defaultRetention}
	rows, err := q.QueryContext(ctx,
		`SELECT key, value FROM settings WHERE key IN (?, ?)`, settingNewPerDay, settingRetention)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) != nil {
			continue
		}
		switch k {
		case settingNewPerDay:
			if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 500 {
				out.NewPerDay = n
			}
		case settingRetention:
			if r, err := strconv.ParseFloat(v, 64); err == nil && r >= 0.7 && r <= 0.99 {
				out.Retention = r
			}
		}
	}
	return out
}

// Settings renvoie les réglages courants (bornés).
func (s *Service) Settings(ctx context.Context) Settings { return s.settings(ctx, s.db) }

func (s *Service) scheduler(set Settings) *fsrs.FSRS {
	p := fsrs.DefaultParam()
	p.RequestRetention = set.Retention
	p.EnableFuzz = true // évite que des cartes apprises ensemble reviennent ensemble
	return fsrs.NewFSRS(p)
}

// Synchronisation avec les notes ----------------------------------------------

// SyncNote aligne les cartes d'une note sur son contenu. À appeler dans la
// transaction qui écrit la note. Une carte existante garde sa progression
// (seul son texte est rafraîchi) ; une carte dont le texte a disparu est mise
// de côté ; une carte qui réapparaît est réactivée telle qu'elle était.
func SyncNote(ctx context.Context, q dbtx, noteID, content string, now time.Time) error {
	drafts := Parse(content)

	existing := map[string]sql.NullString{} // clé → orphaned_at
	rows, err := q.QueryContext(ctx, `SELECT key, orphaned_at FROM cards WHERE note_id = ?`, noteID)
	if err != nil {
		return fmt.Errorf("lecture des cartes : %w", err)
	}
	for rows.Next() {
		var key string
		var orphan sql.NullString
		if err := rows.Scan(&key, &orphan); err != nil {
			rows.Close()
			return fmt.Errorf("lecture des cartes : %w", err)
		}
		existing[key] = orphan
	}
	if err := rows.Close(); err != nil {
		return err
	}

	stamp := now.UTC().Format(tsFormat)
	keep := make(map[string]bool, len(drafts))
	for _, d := range drafts {
		keep[d.Key] = true
		if _, ok := existing[d.Key]; ok {
			if _, err := q.ExecContext(ctx,
				`UPDATE cards SET front = ?, back = ?, kind = ?, orphaned_at = NULL WHERE note_id = ? AND key = ?`,
				d.Front, d.Back, string(d.Kind), noteID, d.Key); err != nil {
				return fmt.Errorf("mise à jour de carte : %w", err)
			}
			continue
		}
		fresh := fsrs.NewCard(now.UTC())
		if _, err := q.ExecContext(ctx, `
			INSERT INTO cards (id, note_id, key, kind, front, back, due, stability, difficulty,
			                   scheduled_days, reps, lapses, state, last_review, remaining_steps, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, 0, 0, 0, 0, '', 0, ?)`,
			uuid.NewString(), noteID, d.Key, string(d.Kind), d.Front, d.Back,
			fresh.Due.UTC().Format(tsFormat), stamp); err != nil {
			return fmt.Errorf("création de carte : %w", err)
		}
	}
	for key, orphan := range existing {
		if keep[key] || orphan.Valid {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`UPDATE cards SET orphaned_at = ? WHERE note_id = ? AND key = ?`, stamp, noteID, key); err != nil {
			return fmt.Errorf("mise de côté d'une carte : %w", err)
		}
	}
	return nil
}

// Maintain indexe une fois les notes existantes (installation qui monte en
// version) et purge les cartes mises de côté depuis trop longtemps.
func (s *Service) Maintain(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("maintenance des cartes : %w", err)
	}
	defer tx.Rollback()

	var done string
	_ = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, settingIndexed).Scan(&done)
	if done != "1" {
		rows, err := tx.QueryContext(ctx, `SELECT id, content FROM notes`)
		if err != nil {
			return fmt.Errorf("lecture des notes : %w", err)
		}
		type note struct{ id, content string }
		var all []note
		for rows.Next() {
			var n note
			if err := rows.Scan(&n.id, &n.content); err != nil {
				rows.Close()
				return err
			}
			all = append(all, n)
		}
		rows.Close()
		now := s.now()
		for _, n := range all {
			if !strings.Contains(n.content, "::") {
				continue
			}
			if err := SyncNote(ctx, tx, n.id, n.content, now); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings(key, value) VALUES(?, '1') ON CONFLICT(key) DO UPDATE SET value = '1'`,
			settingIndexed); err != nil {
			return err
		}
	}

	cutoff := s.now().Add(-orphanGrace).UTC().Format(tsFormat)
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM cards WHERE orphaned_at IS NOT NULL AND orphaned_at < ?`, cutoff); err != nil {
		return fmt.Errorf("purge des cartes : %w", err)
	}
	return tx.Commit()
}

// Cartes ------------------------------------------------------------------------

// Intervals sont les délais proposés pour chaque réponse, déjà formulés.
type Intervals struct {
	Again string `json:"again"`
	Hard  string `json:"hard"`
	Good  string `json:"good"`
	Easy  string `json:"easy"`
}

// CardView est la carte telle que l'interface la présente.
type CardView struct {
	ID        string    `json:"id"`
	NoteID    string    `json:"noteId"`
	NoteTitle string    `json:"noteTitle"`
	Kind      Kind      `json:"kind"`
	Front     string    `json:"front"`
	Back      string    `json:"back"`
	State     int       `json:"state"` // 0 nouvelle, 1 apprentissage, 2 révision, 3 ré-apprentissage
	Reps      int       `json:"reps"`
	Intervals Intervals `json:"intervals"`
}

type cardRow struct {
	id, noteID, noteTitle, key string
	kind                       Kind
	front, back                string
	card                       fsrs.Card
}

const cardColumns = `c.id, c.note_id, n.title, c.key, c.kind, c.front, c.back, c.due, c.stability,
	c.difficulty, c.scheduled_days, c.reps, c.lapses, c.state, c.last_review, c.remaining_steps`

func parseTS(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse(tsFormat, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

func scanCard(r interface{ Scan(...any) error }) (cardRow, error) {
	var c cardRow
	var kind, due, last string
	var sched, reps, lapses, state, steps int64
	if err := r.Scan(&c.id, &c.noteID, &c.noteTitle, &c.key, &kind, &c.front, &c.back, &due,
		&c.card.Stability, &c.card.Difficulty, &sched, &reps, &lapses, &state, &last, &steps); err != nil {
		return cardRow{}, err
	}
	c.kind = Kind(kind)
	c.card.Due = parseTS(due)
	c.card.LastReview = parseTS(last)
	c.card.ScheduledDays = uint64(sched)
	c.card.Reps = uint64(reps)
	c.card.Lapses = uint64(lapses)
	c.card.State = fsrs.State(state)
	c.card.RemainingSteps = int(steps)
	return c, nil
}

func (s *Service) view(f *fsrs.FSRS, c cardRow, now time.Time) CardView {
	v := CardView{
		ID: c.id, NoteID: c.noteID, NoteTitle: c.noteTitle, Kind: c.kind,
		Front: c.front, Back: c.back, State: int(c.card.State), Reps: int(c.card.Reps),
	}
	if rec, err := f.Repeat(c.card, now); err == nil {
		v.Intervals = Intervals{
			Again: humanize(rec[fsrs.Again].Card.Due.Sub(now)),
			Hard:  humanize(rec[fsrs.Hard].Card.Due.Sub(now)),
			Good:  humanize(rec[fsrs.Good].Card.Due.Sub(now)),
			Easy:  humanize(rec[fsrs.Easy].Card.Due.Sub(now)),
		}
	}
	return v
}

// humanize formule un délai en français : « 10 min », « 3 j », « 2 mois ».
func humanize(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "< 1 min"
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Round(time.Minute)/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h", int(d.Round(time.Hour)/time.Hour))
	case d < 31*24*time.Hour:
		return fmt.Sprintf("%d j", int(d.Round(24*time.Hour)/(24*time.Hour)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%d mois", int(d.Round(30*24*time.Hour)/(30*24*time.Hour)))
	default:
		years := float64(d) / float64(365*24*time.Hour)
		return fmt.Sprintf("%.1f an", years)
	}
}

func endOfDay(now time.Time) time.Time {
	y, m, d := now.Local().Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, now.Location()).UTC()
}

func startOfDay(now time.Time) time.Time {
	y, m, d := now.Local().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Local().Location()).UTC()
}

// introducedToday compte les cartes vues pour la première fois aujourd'hui.
func (s *Service) introducedToday(ctx context.Context, now time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT card_id, MIN(reviewed_at) AS first FROM review_log GROUP BY card_id
		) WHERE first >= ?`, startOfDay(now).Format(tsFormat)).Scan(&n)
	return n, err
}

// Next renvoie la prochaine carte à réviser, ou nil s'il n'y en a plus pour
// l'instant. Ordre : apprentissage échu, révisions du jour, nouvelles cartes
// (dans la limite quotidienne), puis apprentissage proche.
func (s *Service) Next(ctx context.Context) (*CardView, error) {
	now := s.now().UTC()
	set := s.settings(ctx, s.db)
	intro, err := s.introducedToday(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("cartes du jour : %w", err)
	}

	queries := []struct {
		where string
		args  []any
		skip  bool
	}{
		{`c.state IN (1, 3) AND c.due <= ?`, []any{now.Format(tsFormat)}, false},
		{`c.state = 2 AND c.due <= ?`, []any{endOfDay(now).Format(tsFormat)}, false},
		{`c.state = 0`, nil, intro >= set.NewPerDay},
		{`c.state IN (1, 3) AND c.due <= ?`, []any{now.Add(learnAhead).Format(tsFormat)}, false},
	}
	for _, q := range queries {
		if q.skip {
			continue
		}
		order := "c.due, c.id"
		if q.where == `c.state = 0` {
			order = "c.created_at, c.key, c.id"
		}
		row := s.db.QueryRowContext(ctx,
			`SELECT `+cardColumns+` FROM cards c JOIN notes n ON n.id = c.note_id
			  WHERE c.orphaned_at IS NULL AND `+q.where+` ORDER BY `+order+` LIMIT 1`, q.args...)
		c, err := scanCard(row)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("lecture de la carte : %w", err)
		}
		v := s.view(s.scheduler(set), c, now)
		return &v, nil
	}
	return nil, nil
}

type beforeSnapshot struct {
	Due            string  `json:"due"`
	Stability      float64 `json:"stability"`
	Difficulty     float64 `json:"difficulty"`
	ScheduledDays  int64   `json:"scheduledDays"`
	Reps           int64   `json:"reps"`
	Lapses         int64   `json:"lapses"`
	State          int64   `json:"state"`
	LastReview     string  `json:"lastReview"`
	RemainingSteps int64   `json:"remainingSteps"`
}

// Answer enregistre une réponse (1 = À revoir, 2 = Difficile, 3 = Bien,
// 4 = Facile) et planifie la carte. L'état précédent est conservé pour
// pouvoir annuler.
func (s *Service) Answer(ctx context.Context, cardID string, rating int) error {
	if rating < int(fsrs.Again) || rating > int(fsrs.Easy) {
		return fmt.Errorf("réponse invalide : %d", rating)
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("réponse : %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx,
		`SELECT `+cardColumns+` FROM cards c JOIN notes n ON n.id = c.note_id WHERE c.id = ?`, cardID)
	c, err := scanCard(row)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("carte %q introuvable", cardID)
	}
	if err != nil {
		return fmt.Errorf("lecture de la carte : %w", err)
	}

	set := s.settings(ctx, tx)
	info, err := s.scheduler(set).Next(c.card, now, fsrs.Rating(rating))
	if err != nil {
		return fmt.Errorf("planification : %w", err)
	}
	before, _ := json.Marshal(beforeSnapshot{
		Due: c.card.Due.UTC().Format(tsFormat), Stability: c.card.Stability, Difficulty: c.card.Difficulty,
		ScheduledDays: int64(c.card.ScheduledDays), Reps: int64(c.card.Reps), Lapses: int64(c.card.Lapses),
		State: int64(c.card.State), LastReview: formatTS(c.card.LastReview), RemainingSteps: int64(c.card.RemainingSteps),
	})
	n := info.Card
	if _, err := tx.ExecContext(ctx, `
		UPDATE cards SET due = ?, stability = ?, difficulty = ?, scheduled_days = ?, reps = ?,
		       lapses = ?, state = ?, last_review = ?, remaining_steps = ? WHERE id = ?`,
		n.Due.UTC().Format(tsFormat), n.Stability, n.Difficulty, int64(n.ScheduledDays), int64(n.Reps),
		int64(n.Lapses), int64(n.State), formatTS(n.LastReview), int64(n.RemainingSteps), cardID); err != nil {
		return fmt.Errorf("enregistrement de la carte : %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO review_log (id, card_id, rating, reviewed_at, state, before) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), cardID, rating, now.Format(tsFormat), int64(c.card.State), string(before)); err != nil {
		return fmt.Errorf("historique : %w", err)
	}
	return tx.Commit()
}

func formatTS(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(tsFormat)
}

// Undo annule la toute dernière réponse et renvoie la carte concernée.
func (s *Service) Undo(ctx context.Context) (*CardView, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("annulation : %w", err)
	}
	defer tx.Rollback()

	var logID, cardID, raw string
	err = tx.QueryRowContext(ctx,
		`SELECT id, card_id, before FROM review_log ORDER BY reviewed_at DESC, rowid DESC LIMIT 1`).
		Scan(&logID, &cardID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("annulation : %w", err)
	}
	var b beforeSnapshot
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		return nil, fmt.Errorf("annulation : état précédent illisible : %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE cards SET due = ?, stability = ?, difficulty = ?, scheduled_days = ?, reps = ?,
		       lapses = ?, state = ?, last_review = ?, remaining_steps = ? WHERE id = ?`,
		b.Due, b.Stability, b.Difficulty, b.ScheduledDays, b.Reps, b.Lapses, b.State, b.LastReview,
		b.RemainingSteps, cardID); err != nil {
		return nil, fmt.Errorf("annulation : %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM review_log WHERE id = ?`, logID); err != nil {
		return nil, fmt.Errorf("annulation : %w", err)
	}
	row := tx.QueryRowContext(ctx,
		`SELECT `+cardColumns+` FROM cards c JOIN notes n ON n.id = c.note_id WHERE c.id = ?`, cardID)
	c, err := scanCard(row)
	if err != nil {
		return nil, fmt.Errorf("annulation : %w", err)
	}
	set := s.settings(ctx, tx)
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("annulation : %w", err)
	}
	v := s.view(s.scheduler(set), c, s.now().UTC())
	return &v, nil
}

// Statistiques ------------------------------------------------------------------

// Stats résume l'état de la révision pour l'accueil et la barre latérale.
type Stats struct {
	Due           int    `json:"due"`           // à réviser maintenant
	New           int    `json:"new"`           // nouvelles cartes disponibles aujourd'hui
	Total         int    `json:"total"`         // cartes actives
	ReviewedToday int    `json:"reviewedToday"` // réponses données aujourd'hui
	NextDue       string `json:"nextDue"`       // prochaine échéance (RFC 3339), si rien n'est dû
	Streak        int    `json:"streak"`        // jours consécutifs avec au moins une réponse
	Activity      []int  `json:"activity"`      // réponses par jour, 14 derniers jours (ancien → récent)
}

func (s *Service) Stats(ctx context.Context) (Stats, error) {
	now := s.now().UTC()
	set := s.settings(ctx, s.db)
	var st Stats

	var learning, reviews, fresh int
	err := s.db.QueryRowContext(ctx, `
		SELECT
		  COALESCE(SUM(CASE WHEN state IN (1,3) AND due <= ? THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN state = 2 AND due <= ? THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN state = 0 THEN 1 ELSE 0 END), 0),
		  COUNT(*)
		FROM cards WHERE orphaned_at IS NULL`,
		now.Add(learnAhead).Format(tsFormat), endOfDay(now).Format(tsFormat)).
		Scan(&learning, &reviews, &fresh, &st.Total)
	if err != nil {
		return st, fmt.Errorf("statistiques : %w", err)
	}
	intro, err := s.introducedToday(ctx, now)
	if err != nil {
		return st, fmt.Errorf("statistiques : %w", err)
	}
	allowed := set.NewPerDay - intro
	if allowed < 0 {
		allowed = 0
	}
	if fresh > allowed {
		fresh = allowed
	}
	st.Due = learning + reviews
	st.New = fresh

	if st.Due == 0 {
		var next sql.NullString
		_ = s.db.QueryRowContext(ctx,
			`SELECT MIN(due) FROM cards WHERE orphaned_at IS NULL AND state <> 0`).Scan(&next)
		if next.Valid {
			st.NextDue = parseTS(next.String).Format(time.RFC3339)
		}
	}

	// Activité : réponses par jour local sur 14 jours.
	const days = 14
	st.Activity = make([]int, days)
	today := startOfDay(now).Local()
	since := today.AddDate(0, 0, -(days - 1)).UTC().Format(tsFormat)
	rows, err := s.db.QueryContext(ctx, `SELECT reviewed_at FROM review_log WHERE reviewed_at >= ?`, since)
	if err != nil {
		return st, fmt.Errorf("activité : %w", err)
	}
	defer rows.Close()
	perDay := map[string]int{}
	for rows.Next() {
		var at string
		if rows.Scan(&at) != nil {
			continue
		}
		perDay[parseTS(at).Local().Format("2006-01-02")]++
	}
	for i := 0; i < days; i++ {
		d := today.AddDate(0, 0, -(days-1)+i).Format("2006-01-02")
		st.Activity[i] = perDay[d]
	}
	st.ReviewedToday = st.Activity[days-1]
	for i := days - 1; i >= 0; i-- {
		if st.Activity[i] > 0 {
			st.Streak++
		} else if i == days-1 {
			continue // aujourd'hui n'est pas fini : la série d'hier compte
		} else {
			break
		}
	}
	return st, nil
}

// Maîtrise ----------------------------------------------------------------------

// MasteryRow décrit la maîtrise d'un dossier ou d'une étiquette.
type MasteryRow struct {
	Kind      string  `json:"kind"` // « folder » | « tag »
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	Cards     int     `json:"cards"`
	Learned   int     `json:"learned"`
	Due       int     `json:"due"`
	Retention float64 `json:"retention"` // moyenne de la probabilité de rappel, 0..1
}

// Mastery estime, par dossier et par étiquette, la probabilité moyenne de se
// souvenir des cartes déjà apprises (probabilité de rappel FSRS, à l'instant).
func (s *Service) Mastery(ctx context.Context) ([]MasteryRow, error) {
	now := s.now().UTC()
	f := s.scheduler(s.settings(ctx, s.db))
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+cardColumns+`, COALESCE(f.id, ''), COALESCE(f.name, ''), n.tags_text
		  FROM cards c
		  JOIN notes n ON n.id = c.note_id
		  LEFT JOIN folders f ON f.id = n.folder_id
		 WHERE c.orphaned_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("maîtrise : %w", err)
	}
	defer rows.Close()

	type agg struct {
		row MasteryRow
		sum float64
	}
	groups := map[string]*agg{}
	add := func(key string, base MasteryRow, learned bool, due bool, r float64) {
		g := groups[key]
		if g == nil {
			g = &agg{row: base}
			groups[key] = g
		}
		g.row.Cards++
		if learned {
			g.row.Learned++
			g.sum += r
		}
		if due {
			g.row.Due++
		}
	}
	for rows.Next() {
		var folderID, folderName, tags string
		var kind, due, last string
		var c cardRow
		var sched, reps, lapses, state, steps int64
		if err := rows.Scan(&c.id, &c.noteID, &c.noteTitle, &c.key, &kind, &c.front, &c.back, &due,
			&c.card.Stability, &c.card.Difficulty, &sched, &reps, &lapses, &state, &last, &steps,
			&folderID, &folderName, &tags); err != nil {
			return nil, fmt.Errorf("maîtrise : %w", err)
		}
		c.card.Due = parseTS(due)
		c.card.LastReview = parseTS(last)
		c.card.State = fsrs.State(state)
		c.card.Reps = uint64(reps)
		learned := c.card.State != fsrs.New
		var r float64
		if learned {
			if v, err := f.Retrievability(c.card, now); err == nil {
				r = v
			}
		}
		isDue := learned && !c.card.Due.After(endOfDay(now))

		if folderID != "" {
			add("f:"+folderID, MasteryRow{Kind: "folder", ID: folderID, Label: folderName}, learned, isDue, r)
		} else {
			add("f:", MasteryRow{Kind: "folder", ID: "", Label: "Sans dossier"}, learned, isDue, r)
		}
		if tags != "" {
			for _, t := range strings.Split(tags, "\x1f") {
				add("t:"+t, MasteryRow{Kind: "tag", ID: t, Label: t}, learned, isDue, r)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]MasteryRow, 0, len(groups))
	for _, g := range groups {
		if g.row.Learned > 0 {
			g.row.Retention = g.sum / float64(g.row.Learned)
		}
		out = append(out, g.row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Due != out[j].Due {
			return out[i].Due > out[j].Due
		}
		return out[i].Label < out[j].Label
	})
	return out, nil
}

// NoteSummary résume les cartes d'une note, pour l'éditeur.
type NoteSummary struct {
	Cards int `json:"cards"`
	Due   int `json:"due"`
	New   int `json:"new"`
}

func (s *Service) NoteSummary(ctx context.Context, noteID string) (NoteSummary, error) {
	var sum NoteSummary
	now := s.now().UTC()
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN state <> 0 AND due <= ? THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN state = 0 THEN 1 ELSE 0 END), 0)
		  FROM cards WHERE note_id = ? AND orphaned_at IS NULL`,
		endOfDay(now).Format(tsFormat), noteID).Scan(&sum.Cards, &sum.Due, &sum.New)
	if err != nil {
		return sum, fmt.Errorf("cartes de la note : %w", err)
	}
	return sum, nil
}

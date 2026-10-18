// Package notes porte le service métier des notes, dossiers et
// étiquettes (cahier des charges §3.1–3.3). Règle d'or : l'index FTS5 est maintenu
// dans la MÊME transaction que la note — un index obsolète est une
// donnée corrompue (ADR-003).
package notes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"notecore/backend/internal/review"
)

// Note reflète le modèle de données du cahier des charges (§7).
// Les dates sont en UTC normalisé (ARCHITECTURE.md §4).
type Note struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	FolderID  *string  `json:"folderId,omitempty"`
	IsPinned  bool     `json:"isPinned"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

type Folder struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Patch décrit les champs modifiables d'une note.
type Patch struct {
	Title    *string
	Content  *string
	FolderID *string // nil = pas de changement ; pointeur vers "" = avancer à la racine
	IsPinned *bool
	Tags     *[]string
}

// SearchResult est ce que l'interface reçoit : extrait déjà structuré
// construit côté backend (ADR-003).
type SearchResult struct {
	NoteID    string `json:"noteId"`
	Title     string `json:"title"`
	Excerpt   string `json:"excerpt"`
	UpdatedAt string `json:"updatedAt"`
}

// Service concentre les règles métier. Il n'expose jamais de handle SQL.
type Service struct {
	db *sql.DB
}

func New(db *sql.DB) *Service {
	return &Service{db: db}
}

// --- lectures -------------------------------------------------------------

func (s *Service) ListNotes(ctx context.Context) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, content, folder_id, is_pinned, tags_text, created_at, updated_at
		FROM notes ORDER BY is_pinned DESC, updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("lecture des notes : %w", err)
	}
	defer rows.Close()
	return collectNotes(rows)
}

func (s *Service) ListFolders(ctx context.Context) ([]Folder, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, created_at, updated_at FROM folders ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("lecture des dossiers : %w", err)
	}
	defer rows.Close()

	out := make([]Folder, 0)
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.Name, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan dossier : %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SearchNotes cherche dans l'index FTS (titre, contenu, étiquettes).
func (s *Service) SearchNotes(ctx context.Context, query string) ([]SearchResult, error) {
	match, err := ftsExpr(query)
	if err != nil {
		return nil, err
	}
	if match == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT notes.id, notes.title,
		       snippet(notes_fts, 1, '«', '»', '…', 12),
		       notes.updated_at
		  FROM notes_fts
		  JOIN notes ON notes.id = notes_fts.note_id
		 WHERE notes_fts MATCH ?
		 ORDER BY bm25(notes_fts), notes.updated_at DESC
		 LIMIT 50`,
		match)
	if err != nil {
		return nil, fmt.Errorf("recherche : %w", err)
	}
	defer rows.Close()

	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.NoteID, &r.Title, &r.Excerpt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan recherche : %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- écritures ------------------------------------------------------------

// CreateNote crée une note. Une note sans titre explicite garde un
// titre par défaut mais reste accessible (cahier des charges §3.1).
func (s *Service) CreateNote(ctx context.Context, title, folderID string) (Note, error) {
	var folderPtr *string
	if folder := strings.TrimSpace(folderID); folder != "" {
		exists, err := folderExists(ctx, s.db, folder)
		if err != nil {
			return Note{}, err
		}
		if !exists {
			return Note{}, fmt.Errorf("dossier %q introuvable", folder)
		}
		folderPtr = &folder
	}

	now := time.Now().UTC().Format(time.RFC3339)
	n := Note{
		ID:        newID(),
		Title:     normalizeTitle(title),
		FolderID:  folderPtr,
		CreatedAt: now,
		UpdatedAt: now,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Note{}, fmt.Errorf("démarrage création : %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO notes (id, title, content, folder_id, is_pinned, tags_text, created_at, updated_at)
		VALUES (?, ?, '', ?, 0, '', ?, ?)`,
		n.ID, n.Title, folderPtr, n.CreatedAt, n.UpdatedAt)
	if err != nil {
		return Note{}, fmt.Errorf("création note : %w", err)
	}
	// Index FTS : même transaction (règle d'or).
	if err := reindexInTx(ctx, tx, n.ID); err != nil {
		return Note{}, err
	}
	if err := tx.Commit(); err != nil {
		return Note{}, fmt.Errorf("commit création : %w", err)
	}
	return n, nil
}

// UpdateNote applique un patch partiel dans une seule transaction :
// note + étiquettes + index FTS sont cohérents d'un seul constat.
func (s *Service) UpdateNote(ctx context.Context, id string, patch Patch) (Note, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Note{}, fmt.Errorf("démarrage mise à jour : %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `
		SELECT id, title, content, folder_id, is_pinned, tags_text, created_at, updated_at
		  FROM notes WHERE id = ?`, id)
	n, err := scanNote(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Note{}, fmt.Errorf("note %q introuvable", id)
		}
		return Note{}, err
	}

	before := n
	if patch.Title != nil {
		// Une note nouvellement créée reçoit « Sans titre », mais l'utilisateur
		// doit pouvoir effacer ce titre puis saisir le sien sans que la réponse
		// d'autosauvegarde ne réinjecte l'ancien libellé.
		n.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.Content != nil {
		n.Content = *patch.Content
	}
	if patch.FolderID != nil {
		folder := strings.TrimSpace(*patch.FolderID)
		if folder == "" {
			n.FolderID = nil
		} else {
			exists, err := folderExists(ctx, tx, folder)
			if err != nil {
				return Note{}, err
			}
			if !exists {
				return Note{}, fmt.Errorf("dossier %q introuvable", folder)
			}
			n.FolderID = &folder
		}
	}
	if patch.IsPinned != nil {
		n.IsPinned = *patch.IsPinned
	}
	if patch.Tags != nil {
		n.Tags = cleanTags(*patch.Tags)
	}

	// La date de modification décrit le contenu : épingler une note ou
	// rejouer une sauvegarde identique ne la fait pas remonter dans « Récentes ».
	if contentChanged(before, n) {
		n.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE notes
		   SET title = ?, content = ?, folder_id = ?, is_pinned = ?, tags_text = ?, updated_at = ?
		 WHERE id = ?`,
		n.Title, n.Content, n.FolderID, n.IsPinned, strings.Join(n.Tags, tagSeparator), n.UpdatedAt, id)
	if err != nil {
		return Note{}, fmt.Errorf("mise à jour note : %w", err)
	}

	if err := syncNoteTags(ctx, tx, id, n.Tags); err != nil {
		return Note{}, err
	}
	// Règle d'or : index FTS recalculé dans cette même transaction.
	if err := reindexInTx(ctx, tx, id); err != nil {
		return Note{}, err
	}
	// Les cartes de révision sont dérivées du texte : elles suivent la note
	// dans la même transaction (ADR-007). Inutile si le contenu n'a pas changé.
	if before.Content != n.Content {
		if err := review.SyncNote(ctx, tx, id, n.Content, time.Now().UTC()); err != nil {
			return Note{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Note{}, fmt.Errorf("commit mise à jour : %w", err)
	}
	return n, nil
}

// DeleteNote supprime sans confirmation (la confirmation est une
// responsabilité d'interface — cahier des charges §3.1). La ligne FTS est
// détachée avant la note, dans la même transaction.
func (s *Service) DeleteNote(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("démarrage suppression : %w", err)
	}
	defer tx.Rollback()

	if err := unindexInTx(ctx, tx, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("suppression note : %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return fmt.Errorf("note %q introuvable", id)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit suppression : %w", err)
	}
	return nil
}

// CreateFolder valide l'unicité insensible à la casse (ARCHITECTURE.md §4).
func (s *Service) CreateFolder(ctx context.Context, name string) (Folder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Folder{}, fmt.Errorf("nom de dossier vide")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	f := Folder{ID: newID(), Name: name, CreatedAt: now, UpdatedAt: now}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO folders (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		f.ID, f.Name, f.CreatedAt, f.UpdatedAt); err != nil {
		if isUniqueConstraint(err) {
			return Folder{}, fmt.Errorf("un dossier « %s » existe déjà (majuscules/minuscules confondues)", name)
		}
		return Folder{}, fmt.Errorf("création dossier : %w", err)
	}
	return f, nil
}

// RenameFolder préserve les notes : seul le nom change.
func (s *Service) RenameFolder(ctx context.Context, id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("nom de dossier vide")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE folders SET name = ?, updated_at = ? WHERE id = ?`,
		name, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		if isUniqueConstraint(err) {
			return fmt.Errorf("un dossier « %s » existe déjà (majuscules/minuscules confondues)", name)
		}
		return fmt.Errorf("renommage dossier : %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return fmt.Errorf("dossier %q introuvable", id)
	}
	return nil
}

// DeleteFolder ne détruit jamais les notes (cahier des charges §3.2) : le FK porte
// ON DELETE SET NULL, chaque note repasse à la racine. Renvoie la
// liste des notes déplacées pour la réponse d'interface.
func (s *Service) DeleteFolder(ctx context.Context, folderID string) ([]Note, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("démarrage suppression dossier : %w", err)
	}
	defer tx.Rollback()

	// Inventaire avant suppression (règle : aucune perte silencieuse).
	moved, err := listNotesByFolderInTx(ctx, tx, folderID)
	if err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM folders WHERE id = ?`, folderID)
	if err != nil {
		return nil, fmt.Errorf("suppression dossier : %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return nil, fmt.Errorf("dossier %q introuvable", folderID)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit suppression dossier : %w", err)
	}
	for i := range moved {
		moved[i].FolderID = nil
	}
	return moved, nil
}

// GetNote lit une note complète (utilisé par l'export).
func (s *Service) GetNote(ctx context.Context, id string) (Note, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, title, content, folder_id, is_pinned, tags_text, created_at, updated_at
		  FROM notes WHERE id = ?`, id)
	n, err := scanNote(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Note{}, fmt.Errorf("note %q introuvable", id)
		}
		return Note{}, err
	}
	return n, nil
}

// --- helpers ------------------------------------------------------------

const tagSeparator = "\x1f" // séparateur interne pour tags_text

// contentChanged dit si la note diffère de son état précédent, hors épinglage.
func contentChanged(a, b Note) bool {
	if a.Title != b.Title || a.Content != b.Content || len(a.Tags) != len(b.Tags) {
		return true
	}
	if (a.FolderID == nil) != (b.FolderID == nil) || (a.FolderID != nil && *a.FolderID != *b.FolderID) {
		return true
	}
	for i := range a.Tags {
		if a.Tags[i] != b.Tags[i] {
			return true
		}
	}
	return false
}

func newID() string {
	// UUID v7 : ordonné dans le temps, généré côté application
	// (ARCHITECTURE.md §4 : identifiants applicatifs, stables à l'export).
	u, err := uuid.NewV7()
	if err != nil {
		u = uuid.New() // repli v4 : l'unicité prime sur l'ordonnancement
	}
	return u.String()
}

func normalizeTitle(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return "Sans titre"
	}
	return t
}

func cleanTags(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	tags := make([]string, 0, len(raw))
	for _, t := range raw {
		t = strings.TrimSpace(t)
		key := strings.ToLower(t)
		if t == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		tags = append(tags, t)
	}
	return tags
}

func splitTags(joined string) []string {
	if joined == "" {
		// Slice non nil : évite `"tags": null` côté JSON (le frontend
		// itère directement sur le tableau).
		return []string{}
	}
	return strings.Split(joined, tagSeparator)
}

func scanNote(r rowScanner) (Note, error) {
	var n Note
	var folderID sql.NullString
	var pinned int
	var tagsText string
	if err := r.Scan(&n.ID, &n.Title, &n.Content, &folderID, &pinned, &tagsText, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return n, fmt.Errorf("scan note : %w", err)
	}
	if folderID.Valid {
		v := folderID.String
		n.FolderID = &v
	}
	n.IsPinned = pinned == 1
	n.Tags = splitTags(tagsText)
	return n, nil
}

type rowScanner interface{ Scan(dest ...any) error }

func collectNotes(rows *sql.Rows) ([]Note, error) {
	out := make([]Note, 0)
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ftsExpr construit une requête MATCH sûre : chaque mot est cité pour
// que les caractères spéciaux FTS de l'utilisateur ne brisent pas la
// syntaxe du moteur. Le dernier mot est un préfixe : la recherche se fait
// pendant la frappe, « arch » doit déjà trouver « architecture ».
func ftsExpr(query string) (string, error) {
	tokens := strings.Fields(strings.TrimSpace(query))
	if len(tokens) == 0 {
		return "", nil
	}
	quoted := make([]string, 0, len(tokens))
	for i, t := range tokens {
		term := `"` + strings.ReplaceAll(t, `"`, " ") + `"`
		if i == len(tokens)-1 {
			term += "*"
		}
		quoted = append(quoted, term)
	}
	return strings.Join(quoted, " "), nil
}

// reindexInTx recalcule la ligne FTS de la note dans la transaction
// passée — jamais hors transaction (règle d'or, journal d'instructions
// unique et cohérent). La ligne précédente est supprimée d'abord :
// FTS5 est une table autonome sans contrainte d'unicité sur note_id,
// donc un simple INSERT empilerait les versions et l'index finirait par
// renvoyer des doublons et d'anciens contenus.
func reindexInTx(ctx context.Context, tx *sql.Tx, noteID string) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM notes_fts WHERE note_id = ?`, noteID); err != nil {
		return fmt.Errorf("réindexation FTS (nettoyage) : %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO notes_fts(note_id, title, content, tags_text)
		 SELECT id, title, content, tags_text FROM notes WHERE id = ?`, noteID); err != nil {
		return fmt.Errorf("indexation FTS : %w", err)
	}
	return nil
}

// unindexInTx retire la ligne FTS de la note, dans la même transaction.
func unindexInTx(ctx context.Context, tx *sql.Tx, noteID string) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM notes_fts WHERE note_id = ?`, noteID); err != nil {
		return fmt.Errorf("désindexation FTS : %w", err)
	}
	return nil
}

// syncNoteTags reconstruit les liens d'étiquettes et crée les
// étiquettes manquantes, dans la même transaction.
func syncNoteTags(ctx context.Context, tx *sql.Tx, noteID string, tags []string) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM note_tags WHERE note_id = ?`, noteID); err != nil {
		return fmt.Errorf("nettoyage étiquettes : %w", err)
	}
	for _, tag := range tags {
		var tagID string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM tags WHERE name = ?`, tag).Scan(&tagID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			tagID = newID()
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO tags (id, name) VALUES (?, ?)`, tagID, tag); err != nil {
				return fmt.Errorf("création étiquette : %w", err)
			}
		case err != nil:
			return fmt.Errorf("lecture étiquette : %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO note_tags (note_id, tag_id) VALUES (?, ?)`,
			noteID, tagID); err != nil {
			return fmt.Errorf("liaison étiquette : %w", err)
		}
	}
	return nil
}

// rowQueryer est satisfait par *sql.DB comme par *sql.Tx. La
// vérification d'existence doit pouvoir s'exécuter DANS la transaction
// appelante : avec un pool d'une seule connexion (ADR-005), interroger
// s.db depuis une transaction ouverte fait attendre la seconde requête
// jusqu'à l'annulation du contexte — un interblocage.
type rowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func folderExists(ctx context.Context, q rowQueryer, id string) (bool, error) {
	var one int
	err := q.QueryRowContext(ctx,
		`SELECT 1 FROM folders WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("vérification dossier : %w", err)
	}
	return true, nil
}

func listNotesByFolderInTx(ctx context.Context, tx *sql.Tx, folderID string) ([]Note, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, title, content, folder_id, is_pinned, tags_text, created_at, updated_at
		  FROM notes WHERE folder_id = ?`, folderID)
	if err != nil {
		return nil, fmt.Errorf("lecture notes du dossier : %w", err)
	}
	defer rows.Close()
	return collectNotes(rows)
}

// isUniqueConstraint reconnaît l'erreur d'unicité SQLite indépendamment
// du pilote (le message moderne contient « UNIQUE constraint failed »).
func isUniqueConstraint(err error) bool {
	return strings.Contains(strings.ToUpper(err.Error()), "UNIQUE")
}

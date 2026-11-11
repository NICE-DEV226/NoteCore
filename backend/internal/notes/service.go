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
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

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
	// DeletedAt : date de mise à la corbeille (RFC 3339), null/absent pour
	// une note active. Une note à la corbeille n'apparaît nulle part
	// (listes, recherche, révision) mais garde tout : ses pièces jointes
	// et ses cartes de révision la suivent, la restauration est complète.
	DeletedAt *string `json:"deletedAt,omitempty"`
}

type Folder struct {
	ID string `json:"id"`
	// ParentID : dossier contenant celui-ci (nil = racine), ADR-009.
	ParentID  *string `json:"parentId,omitempty"`
	Name      string  `json:"name"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
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
	// Kind vaut "attachment" quand le résultat vient de l'index FTS des
	// pièces jointes texte : AttachmentID/Name identifient le fichier,
	// la note hôte reste NoteID.
	Kind           string `json:"kind,omitempty"`
	AttachmentID   string `json:"attachmentId,omitempty"`
	AttachmentName string `json:"attachmentName,omitempty"`
}

// Service concentre les règles métier. Il n'expose jamais de handle SQL.
type Service struct {
	db *sql.DB
}

func New(db *sql.DB) *Service {
	return &Service{db: db}
}

// --- lectures -------------------------------------------------------------

// noteColumns est la liste des colonnes lues par scanNote : tout SELECT
// qui alimente un Note doit la reprendre telle quelle.
const noteColumns = `id, title, content, folder_id, is_pinned, tags_text, created_at, updated_at, deleted_at`

func (s *Service) ListNotes(ctx context.Context) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+noteColumns+`
		FROM notes WHERE deleted_at IS NULL ORDER BY is_pinned DESC, updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("lecture des notes : %w", err)
	}
	defer rows.Close()
	return collectNotes(rows)
}

// searchFilter porte les opérateurs de recherche reconnus dans la
// requête : `tag:<étiquette>` (ET entre elles), `dossier:<nom>` (OU
// entre eux) et le mot isolé `favori`/`épinglé`/`pin`.
type searchFilter struct {
	text    string   // termes libres, pour le moteur FTS
	tags    []string // étiquettes exactes exigées
	folders []string // noms de dossiers acceptés
	pinned  bool     // ne garder que les favoris
}

// parseSearchQuery sépare les opérateurs des termes libres. Un opérateur
// inconnu ou mal formé reste dans le texte : il est cherché tel quel
// plutôt que d'être avalé en silence.
func parseSearchQuery(query string) searchFilter {
	var f searchFilter
	var free []string
	for _, tok := range strings.Fields(query) {
		lower := strings.ToLower(tok)
		switch {
		case strings.HasPrefix(lower, "tag:") && len(tok) > 4:
			f.tags = append(f.tags, strings.ToLower(tok[4:]))
		case (strings.HasPrefix(lower, "dossier:") && len(tok) > 8) ||
			(strings.HasPrefix(lower, "folder:") && len(tok) > 7):
			v := tok
			if strings.HasPrefix(lower, "dossier:") {
				v = tok[8:]
			} else {
				v = tok[7:]
			}
			f.folders = append(f.folders, v)
		case lower == "favori" || lower == "épinglé" || lower == "epingle" || lower == "pin":
			f.pinned = true
		default:
			free = append(free, tok)
		}
	}
	f.text = strings.Join(free, " ")
	return f
}

// SearchNotes cherche dans l'index FTS (titre, contenu, étiquettes),
// affinée par les opérateurs de parseSearchQuery.
func (s *Service) SearchNotes(ctx context.Context, query string) ([]SearchResult, error) {
	f := parseSearchQuery(query)
	if f.text == "" && len(f.tags) == 0 && len(f.folders) == 0 && !f.pinned {
		return nil, nil
	}

	// Clause de filtrage commune aux deux chemins (FTS ou scan direct) :
	// chaque fragment et ses paramètres sont construits ensemble.
	var filters strings.Builder
	var args []any
	for _, tag := range f.tags {
		// tags_text est joint par \x1f (char(31)) : la recherche est
		// exacte, une étiquette « art » ne ramène pas « start ».
		// lower() couvre la casse (les étiquettes ne sont pas
		// normalisées en base).
		filters.WriteString(` AND instr(lower(char(31)||notes.tags_text||char(31)), char(31)||?||char(31)) > 0`)
		args = append(args, tag)
	}
	if len(f.folders) > 0 {
		marks := strings.TrimSuffix(strings.Repeat("?,", len(f.folders)), ",")
		filters.WriteString(` AND notes.folder_id IN (SELECT id FROM folders WHERE name IN (` + marks + `) COLLATE NOCASE)`)
		for _, name := range f.folders {
			args = append(args, name)
		}
	}
	if f.pinned {
		filters.WriteString(` AND notes.is_pinned = 1`)
	}

	var attHits []SearchResult
	var rows *sql.Rows
	var err error
	if f.text != "" {
		match, ferr := ftsExpr(f.text)
		if ferr != nil {
			return nil, ferr
		}
		// Les pièces jointes texte indexées (schéma 7) ne sont cherchées
		// que sur le texte libre : leurs résultats passent AVANT ceux des
		// notes pour que, à note identique, l'extrait de note l'emporte.
		attHits, err = s.searchAttachmentText(ctx, match)
		if err != nil {
			return nil, err
		}
		// Extrait par colonne (titre, contenu, étiquettes) : une correspondance
		// qui ne porte que sur le titre ne doit pas produire un extrait de
		// contenu sans rapport — on retient le premier extrait qui surligne
		// réellement le terme cherché.
		rows, err = s.db.QueryContext(ctx, `
			SELECT notes.id, notes.title,
			       snippet(notes_fts, 1, '«', '»', '…', 12),
			       snippet(notes_fts, 0, '«', '»', '…', 12),
			       snippet(notes_fts, 2, '«', '»', '…', 12),
			       notes.updated_at
			  FROM notes_fts
			  JOIN notes ON notes.id = notes_fts.note_id
			 WHERE notes_fts MATCH ? AND notes.deleted_at IS NULL`+filters.String()+`
			 ORDER BY bm25(notes_fts), notes.updated_at DESC
			 LIMIT 50`,
			append([]any{match}, args...)...)
	} else {
		// Recherche par opérateurs seuls : pas de texte libre, donc pas de
		// jointure FTS — un balayage borné des notes actives suffit.
		rows, err = s.db.QueryContext(ctx, `
			SELECT notes.id, notes.title, '', '', '', notes.updated_at
			  FROM notes
			 WHERE notes.deleted_at IS NULL`+filters.String()+`
			 ORDER BY notes.is_pinned DESC, notes.updated_at DESC
			 LIMIT 50`, args...)
	}
	if err != nil {
		return nil, fmt.Errorf("recherche : %w", err)
	}
	defer rows.Close()

	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		var snippets [3]string
		if err := rows.Scan(&r.NoteID, &r.Title, &snippets[0], &snippets[1], &snippets[2], &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan recherche : %w", err)
		}
		r.Excerpt = bestSnippet(snippets)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return append(attHits, out...), nil
}

// bestSnippet choisit l'extrait le plus parlant : celui qui contient les
// balises de surlignage «…», dans l'ordre contenu, titre, étiquettes ;
// à défaut, le premier extrait non vide.
func bestSnippet(snippets [3]string) string {
	for _, s := range snippets {
		if strings.Contains(s, "«") {
			return s
		}
	}
	for _, s := range snippets {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
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
		SELECT `+noteColumns+` FROM notes WHERE id = ?`, id)
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
	if before.Title != n.Title || before.Content != n.Content {
		if err := recordVersionInTx(ctx, tx, id, n.Title, n.Content); err != nil {
			return Note{}, err
		}
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

// DeleteNote met la note à la corbeille : elle disparaît des listes, de la
// recherche et de la révision, mais rien n'est détruit — ses pièces
// jointes et ses cartes de révision restent liés et la restauration est
// complète (RestoreNote). La suppression définitive est DeleteNoteForever,
// réservée à la corbeille ou à la purge automatique.
func (s *Service) DeleteNote(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("démarrage suppression : %w", err)
	}
	defer tx.Rollback()

	if err := unindexInTx(ctx, tx, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE notes SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`,
		time.Now().UTC().Format(time.RFC3339), id)
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

// --- corbeille -------------------------------------------------------------

// ListTrashed renvoie les notes à la corbeille, les plus récemment mises
// de côté d'abord.
func (s *Service) ListTrashed(ctx context.Context) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+noteColumns+`
		  FROM notes WHERE deleted_at IS NOT NULL
		 ORDER BY deleted_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("lecture de la corbeille : %w", err)
	}
	defer rows.Close()
	return collectNotes(rows)
}

// RestoreNote sort une note de la corbeille : même identifiant, mêmes
// pièces jointes, mêmes cartes de révision — rien n'a été perdu pendant
// la mise de côté. L'index FTS est reconstruit dans la même transaction.
func (s *Service) RestoreNote(ctx context.Context, id string) (Note, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Note{}, fmt.Errorf("démarrage restauration : %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE notes SET deleted_at = NULL WHERE id = ? AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return Note{}, fmt.Errorf("restauration note : %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return Note{}, fmt.Errorf("note %q introuvable dans la corbeille", id)
	}
	if err := reindexInTx(ctx, tx, id); err != nil {
		return Note{}, err
	}
	if err := tx.Commit(); err != nil {
		return Note{}, fmt.Errorf("commit restauration : %w", err)
	}
	return s.GetNote(ctx, id)
}

// DeleteNoteForever détruit définitivement une note : c'est le SEUL
// chemin où les cascades SQL jouent (pièces jointes, cartes, étiquettes).
// Réservé à la corbeille ou à la purge automatique — jamais à la
// suppression « normale », qui passe par DeleteNote.
func (s *Service) DeleteNoteForever(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("démarrage suppression définitive : %w", err)
	}
	defer tx.Rollback()

	if err := unindexInTx(ctx, tx, id); err != nil {
		return err
	}
	if err := unindexNoteAttachmentsInTx(ctx, tx,
		`SELECT id FROM notes WHERE id = ?`, id); err != nil {
		return fmt.Errorf("nettoyage index pièces jointes : %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("suppression définitive note : %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return fmt.Errorf("note %q introuvable", id)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit suppression définitive : %w", err)
	}
	return nil
}

// EmptyTrash vide la corbeille : chaque note est supprimée définitivement,
// avec tout ce qui lui est lié. Renvoie le nombre de notes détruites.
func (s *Service) EmptyTrash(ctx context.Context) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("démarrage vidage corbeille : %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM notes_fts WHERE note_id IN (SELECT id FROM notes WHERE deleted_at IS NOT NULL)`); err != nil {
		return 0, fmt.Errorf("nettoyage index corbeille : %w", err)
	}
	if err := unindexNoteAttachmentsInTx(ctx, tx,
		`SELECT id FROM notes WHERE deleted_at IS NOT NULL`); err != nil {
		return 0, fmt.Errorf("nettoyage index pièces jointes : %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM notes WHERE deleted_at IS NOT NULL`)
	if err != nil {
		return 0, fmt.Errorf("vidage corbeille : %w", err)
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit vidage corbeille : %w", err)
	}
	return int(n), nil
}

// PurgeTrash détruit définitivement les notes à la corbeille depuis plus
// longtemps que la durée donnée (rétention de la corbeille). Appelé au
// démarrage : une note oubliée finit par être libérée, jamais avant.
// Renvoie le nombre de notes purgées.
func (s *Service) PurgeTrash(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("démarrage purge corbeille : %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM notes_fts WHERE note_id IN (SELECT id FROM notes WHERE deleted_at IS NOT NULL AND deleted_at < ?)`,
		cutoff); err != nil {
		return 0, fmt.Errorf("nettoyage index corbeille : %w", err)
	}
	if err := unindexNoteAttachmentsInTx(ctx, tx,
		`SELECT id FROM notes WHERE deleted_at IS NOT NULL AND deleted_at < ?`, cutoff); err != nil {
		return 0, fmt.Errorf("nettoyage index pièces jointes : %w", err)
	}
	res, err := tx.ExecContext(ctx,
		`DELETE FROM notes WHERE deleted_at IS NOT NULL AND deleted_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge corbeille : %w", err)
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit purge corbeille : %w", err)
	}
	return int(n), nil
}

// GetNote lit une note complète (utilisé par l'export). Une note à la
// corbeille reste lisible par identifiant : la restauration et l'export
// en dépendent.
func (s *Service) GetNote(ctx context.Context, id string) (Note, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+noteColumns+` FROM notes WHERE id = ?`, id)
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

// contentChanged dit si la note diffère de son état précédent, hors épinglage
// et hors classement : ranger une note dans un dossier est de l'organisation,
// pas une modification de son contenu (ADR-009).
func contentChanged(a, b Note) bool {
	if a.Title != b.Title || a.Content != b.Content || len(a.Tags) != len(b.Tags) {
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
	var folderID, deletedAt sql.NullString
	var pinned int
	var tagsText string
	if err := r.Scan(&n.ID, &n.Title, &n.Content, &folderID, &pinned, &tagsText, &n.CreatedAt, &n.UpdatedAt, &deletedAt); err != nil {
		return n, fmt.Errorf("scan note : %w", err)
	}
	if folderID.Valid {
		v := folderID.String
		n.FolderID = &v
	}
	if deletedAt.Valid {
		v := deletedAt.String
		n.DeletedAt = &v
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
		SELECT `+noteColumns+`
		  FROM notes WHERE folder_id = ? AND deleted_at IS NULL`, folderID)
	if err != nil {
		return nil, fmt.Errorf("lecture notes du dossier : %w", err)
	}
	defer rows.Close()
	return collectNotes(rows)
}

// isUniqueConstraint reconnaît une violation d'unicité par le code
// étendu SQLite (le driver modernc active les codes étendus), avec un
// repli sur le message pour les erreurs non typées.
func isUniqueConstraint(err error) bool {
	var se *sqlite.Error
	if errors.As(err, &se) {
		return se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE ||
			se.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
	}
	return strings.Contains(strings.ToUpper(err.Error()), "UNIQUE")
}

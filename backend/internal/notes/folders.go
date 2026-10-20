// Dossiers imbriqués, déplacement groupé (ADR-009). Règles de fond :
//   - ranger ne détruit jamais rien : supprimer un dossier remonte ses notes et
//     ses sous-dossiers d'un niveau ;
//   - le nom d'un dossier est unique parmi les dossiers de même parent ;
//   - un dossier ne peut pas être déplacé dans l'un de ses descendants ;
//   - un déplacement groupé est tout-ou-rien.
package notes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MaxFolderDepth borne la profondeur de l'arborescence : au-delà, elle ne se
// lit plus, et une boucle de données corrompues ne peut pas faire tourner à vide.
const MaxFolderDepth = 8

// ListFolders renvoie tous les dossiers (à plat, avec leur parent), triés par nom.
func (s *Service) ListFolders(ctx context.Context) ([]Folder, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, parent_id, name, created_at, updated_at FROM folders ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("lecture des dossiers : %w", err)
	}
	defer rows.Close()

	out := make([]Folder, 0)
	for rows.Next() {
		var f Folder
		var parent sql.NullString
		if err := rows.Scan(&f.ID, &parent, &f.Name, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan dossier : %w", err)
		}
		if parent.Valid {
			v := parent.String
			f.ParentID = &v
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// folderParents lit « id → parent » dans la transaction donnée.
func folderParents(ctx context.Context, q interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}) (map[string]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, COALESCE(parent_id, '') FROM folders`)
	if err != nil {
		return nil, fmt.Errorf("lecture des dossiers : %w", err)
	}
	defer rows.Close()
	parents := make(map[string]string)
	for rows.Next() {
		var id, parent string
		if err := rows.Scan(&id, &parent); err != nil {
			return nil, err
		}
		parents[id] = parent
	}
	return parents, rows.Err()
}

// depthOf compte les niveaux de la racine jusqu'à id (un dossier à la racine : 1).
func depthOf(parents map[string]string, id string) int {
	depth := 0
	for cur := id; cur != "" && depth <= MaxFolderDepth+1; cur = parents[cur] {
		depth++
	}
	return depth
}

// subtreeHeight : hauteur du sous-arbre de id (un dossier sans enfant : 1).
func subtreeHeight(parents map[string]string, id string) int {
	best := 1
	for child, parent := range parents {
		if parent == id {
			if h := 1 + subtreeHeight(parents, child); h > best {
				best = h
			}
		}
	}
	return best
}

func isDescendant(parents map[string]string, candidate, ancestor string) bool {
	for cur, n := candidate, 0; cur != "" && n <= MaxFolderDepth+2; cur, n = parents[cur], n+1 {
		if cur == ancestor {
			return true
		}
	}
	return false
}

func uniqueFolderMessage(name string) error {
	return fmt.Errorf("un dossier « %s » existe déjà à cet endroit (majuscules/minuscules confondues)", name)
}

// CreateFolder crée un dossier à la racine.
func (s *Service) CreateFolder(ctx context.Context, name string) (Folder, error) {
	return s.CreateFolderIn(ctx, name, "")
}

// CreateFolderIn crée un dossier dans parentID ("" = racine).
func (s *Service) CreateFolderIn(ctx context.Context, name, parentID string) (Folder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Folder{}, fmt.Errorf("nom de dossier vide")
	}
	parentID = strings.TrimSpace(parentID)

	if parentID != "" {
		parents, err := folderParents(ctx, s.db)
		if err != nil {
			return Folder{}, err
		}
		if _, ok := parents[parentID]; !ok {
			return Folder{}, fmt.Errorf("dossier parent %q introuvable", parentID)
		}
		if depthOf(parents, parentID) >= MaxFolderDepth {
			return Folder{}, fmt.Errorf("profondeur maximale atteinte (%d niveaux)", MaxFolderDepth)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	f := Folder{ID: newID(), Name: name, CreatedAt: now, UpdatedAt: now}
	var parentArg any
	if parentID != "" {
		f.ParentID = &parentID
		parentArg = parentID
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO folders (id, parent_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		f.ID, parentArg, f.Name, f.CreatedAt, f.UpdatedAt); err != nil {
		if isUniqueConstraint(err) {
			return Folder{}, uniqueFolderMessage(name)
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
			return uniqueFolderMessage(name)
		}
		return fmt.Errorf("renommage dossier : %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return fmt.Errorf("dossier %q introuvable", id)
	}
	return nil
}

// MoveFolder place un dossier (et tout ce qu'il contient) sous newParentID ("" =
// racine). Refusé : vers lui-même ou l'un de ses descendants, au-delà de la
// profondeur maximale, ou si un dossier de même nom existe déjà à destination.
func (s *Service) MoveFolder(ctx context.Context, id, newParentID string) error {
	newParentID = strings.TrimSpace(newParentID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("déplacement de dossier : %w", err)
	}
	defer tx.Rollback()

	parents, err := folderParents(ctx, tx)
	if err != nil {
		return err
	}
	if _, ok := parents[id]; !ok {
		return fmt.Errorf("dossier %q introuvable", id)
	}
	if newParentID != "" {
		if _, ok := parents[newParentID]; !ok {
			return fmt.Errorf("dossier de destination %q introuvable", newParentID)
		}
		if newParentID == id || isDescendant(parents, newParentID, id) {
			return errors.New("un dossier ne peut pas être déplacé dans lui-même ni dans l'un de ses sous-dossiers")
		}
		if depthOf(parents, newParentID)+subtreeHeight(parents, id) > MaxFolderDepth {
			return fmt.Errorf("profondeur maximale dépassée (%d niveaux)", MaxFolderDepth)
		}
	}
	if parents[id] == newParentID {
		return nil // déjà à cet endroit
	}

	var parentArg any
	if newParentID != "" {
		parentArg = newParentID
	}
	if _, err := tx.ExecContext(ctx, `UPDATE folders SET parent_id = ?, updated_at = ? WHERE id = ?`,
		parentArg, time.Now().UTC().Format(time.RFC3339), id); err != nil {
		if isUniqueConstraint(err) {
			var name string
			_ = tx.QueryRowContext(ctx, `SELECT name FROM folders WHERE id = ?`, id).Scan(&name)
			return uniqueFolderMessage(name)
		}
		return fmt.Errorf("déplacement de dossier : %w", err)
	}
	return tx.Commit()
}

// DeleteFolder ne détruit jamais rien (cahier des charges §3.2) : les notes
// du dossier et ses sous-dossiers montent d'un niveau, chez le parent (ou à la
// racine s'il n'en a pas). Renvoie les notes déplacées, avec leur nouveau
// dossier, pour l'interface.
func (s *Service) DeleteFolder(ctx context.Context, folderID string) ([]Note, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("démarrage suppression dossier : %w", err)
	}
	defer tx.Rollback()

	var parent sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT parent_id FROM folders WHERE id = ?`, folderID).Scan(&parent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("dossier %q introuvable", folderID)
	}
	if err != nil {
		return nil, fmt.Errorf("lecture du dossier : %w", err)
	}

	// Inventaire avant suppression (règle : aucune perte silencieuse).
	moved, err := listNotesByFolderInTx(ctx, tx, folderID)
	if err != nil {
		return nil, err
	}

	var parentArg any
	if parent.Valid {
		parentArg = parent.String
	}
	// Les sous-dossiers montent d'un niveau. Un nom déjà pris chez le parent
	// ferait échouer l'opération : on le dit, plutôt que de renommer en silence.
	if _, err := tx.ExecContext(ctx, `UPDATE folders SET parent_id = ? WHERE parent_id = ?`, parentArg, folderID); err != nil {
		if isUniqueConstraint(err) {
			return nil, errors.New("un sous-dossier porte le même nom qu'un dossier du niveau au-dessus : renommez-l'un des deux avant de supprimer ce dossier")
		}
		return nil, fmt.Errorf("remontée des sous-dossiers : %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notes SET folder_id = ? WHERE folder_id = ?`, parentArg, folderID); err != nil {
		return nil, fmt.Errorf("remontée des notes : %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM folders WHERE id = ?`, folderID); err != nil {
		return nil, fmt.Errorf("suppression dossier : %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit suppression dossier : %w", err)
	}
	for i := range moved {
		if parent.Valid {
			v := parent.String
			moved[i].FolderID = &v
		} else {
			moved[i].FolderID = nil
		}
	}
	return moved, nil
}

// MoveNotes range plusieurs notes dans un dossier ("" = sans dossier), tout ou
// rien. Ranger ne modifie pas la date de modification des notes.
func (s *Service) MoveNotes(ctx context.Context, noteIDs []string, folderID string) error {
	if len(noteIDs) == 0 {
		return nil
	}
	if len(noteIDs) > 5000 {
		return errors.New("trop de notes à la fois (maximum 5000)")
	}
	folderID = strings.TrimSpace(folderID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("déplacement des notes : %w", err)
	}
	defer tx.Rollback()

	var folderArg any
	if folderID != "" {
		exists, err := folderExists(ctx, tx, folderID)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("dossier %q introuvable", folderID)
		}
		folderArg = folderID
	}
	for _, id := range noteIDs {
		res, err := tx.ExecContext(ctx, `UPDATE notes SET folder_id = ? WHERE id = ?`, folderArg, id)
		if err != nil {
			return fmt.Errorf("déplacement de la note %q : %w", id, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("note %q introuvable : rien n'a été déplacé", id)
		}
	}
	return tx.Commit()
}

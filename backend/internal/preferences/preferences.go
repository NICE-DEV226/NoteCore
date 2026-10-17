// Package preferences conserve les réglages locaux de l'utilisateur dans
// la table `settings` de la base (cahier des charges §3.6, ADR-002). Le réglage est
// donc persisté avec les données qu'il accompagne : il survit au
// redémarrage sans dépendre du stockage du moteur d'affichage.
package preferences

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Bornes de défense : un réglage est un couple de textes courts. On
// refuse tôt toute entrée hors bornes plutôt que de laisser la table
// servir de stockage de fichiers.
const (
	maxKeyLen   = 64
	maxValueLen = 512
)

// Store expose les réglages persistés.
type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	if db == nil {
		panic("preferences : base absente")
	}
	return &Store{db: db}
}

// validate refuse les entrées hors bornes.
func validate(key, value string) error {
	if key == "" {
		return errors.New("clé de réglage vide")
	}
	if len(key) > maxKeyLen {
		return fmt.Errorf("clé de réglage trop longue (max %d)", maxKeyLen)
	}
	if len(value) > maxValueLen {
		return fmt.Errorf("valeur de réglage trop longue (max %d)", maxValueLen)
	}
	return nil
}

// Get renvoie la valeur d'un réglage. La chaîne vide signifie « non
// défini » : l'interface applique alors sa valeur par défaut.
func (s *Store) Get(ctx context.Context, key string) (string, error) {
	key = strings.TrimSpace(key)
	if err := validate(key, ""); err != nil {
		return "", err
	}

	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lecture du réglage %q : %w", key, err)
	}
	return value, nil
}

// All renvoie l'ensemble des réglages : l'interface hydrate son état en
// un seul aller-retour au démarrage.
func (s *Store) All(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("lecture des réglages : %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("lecture des réglages : %w", err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lecture des réglages : %w", err)
	}
	return out, nil
}

// Set enregistre un réglage, ou remplace sa valeur s'il existe déjà.
func (s *Store) Set(ctx context.Context, key, value string) error {
	key = strings.TrimSpace(key)
	if err := validate(key, value); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("écriture du réglage %q : %w", key, err)
	}
	return nil
}

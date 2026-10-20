// Package share porte le partage de notes SANS connexion Internet (ADR-008) :
// un paquet `.notecore` qui se transmet par n'importe quel moyen (clé USB,
// Bluetooth, dossier partagé, messagerie locale), un envoi direct sur le réseau
// local, et l'import — des paquets NoteCore comme de simples fichiers Markdown.
//
// Règle de fond : importer n'écrase, ne supprime et ne modifie jamais une note
// existante. Une note déjà présente est ignorée, une note différente de même
// titre est ajoutée à côté.
package share

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// magic ouvre tout paquet protégé par un mot de passe. Il sert aussi de
// données authentifiées : un en-tête altéré fait échouer le déchiffrement.
const magic = "NCSHARE1"

const (
	saltLen = 16
	// 600 000 itérations : valeur de référence OWASP (2023) pour PBKDF2-SHA256.
	kdfIterations = 600_000
)

// Erreurs stables : l'interface s'appuie sur leur texte pour demander le mot
// de passe ou signaler une erreur de saisie.
var (
	ErrPassphraseRequired = errors.New("mot de passe requis : ce fichier est protégé")
	ErrBadPassphrase      = errors.New("mot de passe incorrect, ou fichier altéré")
)

// IsEncrypted dit si les données sont un paquet protégé.
func IsEncrypted(data []byte) bool { return bytes.HasPrefix(data, []byte(magic)) }

func deriveKey(passphrase string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, passphrase, salt, kdfIterations, 32)
}

// Encrypt protège un paquet : AES-256-GCM, clé dérivée du mot de passe par
// PBKDF2-SHA256. Format : magic | sel (16) | nonce (12) | chiffré.
func Encrypt(plain []byte, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, errors.New("mot de passe vide")
	}
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("sel : %w", err)
	}
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, fmt.Errorf("dérivation de la clé : %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce : %w", err)
	}
	out := make([]byte, 0, len(magic)+saltLen+len(nonce)+len(plain)+gcm.Overhead())
	out = append(out, magic...)
	out = append(out, salt...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plain, []byte(magic)), nil
}

// Decrypt ouvre un paquet protégé. Un mauvais mot de passe et un fichier
// altéré donnent la même erreur : GCM ne permet pas de les distinguer.
func Decrypt(data []byte, passphrase string) ([]byte, error) {
	if !IsEncrypted(data) {
		return nil, errors.New("ce fichier n'est pas protégé")
	}
	if passphrase == "" {
		return nil, ErrPassphraseRequired
	}
	header := len(magic) + saltLen
	if len(data) < header+12+16 {
		return nil, ErrBadPassphrase
	}
	salt := data[len(magic):header]
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, fmt.Errorf("dérivation de la clé : %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := data[header : header+gcm.NonceSize()]
	plain, err := gcm.Open(nil, nonce, data[header+gcm.NonceSize():], []byte(magic))
	if err != nil {
		return nil, ErrBadPassphrase
	}
	return plain, nil
}

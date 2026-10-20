// Partage de notes sans connexion Internet et import (ADR-008).
package bindings

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"notecore/backend/internal/share"
)

// ShareOptions décrit ce qu'on partage et comment.
type ShareOptions struct {
	Scope      string `json:"scope"`  // note | folder | all
	ID         string `json:"id"`     // note ou dossier
	Format     string `json:"format"` // notecore | markdown
	Passphrase string `json:"passphrase"`
}

func (o ShareOptions) build(n *NoteCore) (share.Package, error) {
	return share.Build(n.callContext(), n.svc,
		share.Selection{Scope: share.Scope(o.Scope), ID: o.ID},
		share.Format(o.Format), o.Passphrase, time.Now())
}

// ShareSaved est le résultat d'un enregistrement de paquet.
type ShareSaved struct {
	Saved       bool   `json:"saved"` // false : l'utilisateur a annulé
	Path        string `json:"path"`
	Notes       int    `json:"notes"`
	Attachments int    `json:"attachments"`
	Encrypted   bool   `json:"encrypted"`
}

// ShareToFile enregistre un paquet à l'endroit choisi : clé USB, dossier
// partagé, Bluetooth… tout moyen de transmettre un fichier convient.
func (n *NoteCore) ShareToFile(opt ShareOptions) (ShareSaved, error) {
	pkg, err := opt.build(n)
	if err != nil {
		return ShareSaved{}, err
	}
	filter := wailsruntime.FileFilter{DisplayName: "Paquet NoteCore (*.notecore)", Pattern: "*.notecore"}
	if opt.Format == string(share.FormatMarkdown) {
		filter = wailsruntime.FileFilter{DisplayName: "Archive Markdown (*.zip)", Pattern: "*.zip"}
	}
	path, err := wailsruntime.SaveFileDialog(n.callContext(), wailsruntime.SaveDialogOptions{
		Title:           "Enregistrer le paquet à partager",
		DefaultFilename: pkg.Filename,
		Filters:         []wailsruntime.FileFilter{filter},
	})
	if err != nil {
		return ShareSaved{}, fmt.Errorf("dialog d'enregistrement : %w", err)
	}
	if path == "" {
		return ShareSaved{}, nil
	}
	if err := writeFileAtomic(path, pkg.Data); err != nil {
		return ShareSaved{}, err
	}
	return ShareSaved{Saved: true, Path: path, Notes: pkg.Notes, Attachments: pkg.Attachments, Encrypted: pkg.Encrypted}, nil
}

// writeFileAtomic écrit dans un fichier voisin puis renomme : un envoi
// interrompu ne laisse jamais un paquet tronqué sous le nom final.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("écriture du paquet : %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("installation du paquet : %w", err)
	}
	return nil
}

// LanShare décrit l'envoi sur le réseau local qui vient de démarrer.
type LanShare struct {
	share.SessionInfo
	Notes       int `json:"notes"`
	Attachments int `json:"attachments"`
}

// ShareStart ouvre un envoi direct sur le réseau local : un lien et un QR code
// à présenter à l'autre appareil, utilisable une seule fois, quelques minutes.
func (n *NoteCore) ShareStart(opt ShareOptions) (LanShare, error) {
	pkg, err := opt.build(n)
	if err != nil {
		return LanShare{}, err
	}
	info, err := n.shares.Start(pkg.Data, pkg.Filename, pkg.Encrypted, share.DefaultTTL)
	if err != nil {
		return LanShare{}, err
	}
	return LanShare{SessionInfo: info, Notes: pkg.Notes, Attachments: pkg.Attachments}, nil
}

// ShareStatus renvoie l'état de l'envoi (waiting, downloaded, expired, stopped, idle).
func (n *NoteCore) ShareStatus() share.Status { return n.shares.Status() }

// ShareStop ferme l'envoi en cours.
func (n *NoteCore) ShareStop() { n.shares.Stop() }

// ImportResult est le résultat d'un import. Un paquet protégé n'est pas une
// erreur : l'interface demande le mot de passe puis rappelle ImportPending
// (un lien d'envoi ne se télécharge qu'une fois, il ne faut pas le perdre).
type ImportResult struct {
	Cancelled       bool         `json:"cancelled"`
	NeedsPassphrase bool         `json:"needsPassphrase"`
	WrongPassphrase bool         `json:"wrongPassphrase"` // une saisie vient d'être refusée
	PendingID       string       `json:"pendingId"`
	Report          share.Report `json:"report"`
}

type pendingImport struct {
	id   string
	data []byte
}

func (n *NoteCore) runImport(data []byte, passphrase, pendingID string) (ImportResult, error) {
	rep, err := share.Import(n.callContext(), n.svc, data, passphrase)
	needs, wrong := errors.Is(err, share.ErrPassphraseRequired), errors.Is(err, share.ErrBadPassphrase)
	if needs || wrong {
		id := pendingID
		if id == "" {
			var raw [8]byte
			_, _ = rand.Read(raw[:])
			id = hex.EncodeToString(raw[:])
		}
		n.mu.Lock()
		n.pending = &pendingImport{id: id, data: data}
		n.mu.Unlock()
		return ImportResult{NeedsPassphrase: true, WrongPassphrase: wrong, PendingID: id}, nil
	}
	if err != nil {
		return ImportResult{}, err
	}
	n.mu.Lock()
	n.pending = nil
	n.mu.Unlock()
	return ImportResult{Report: rep}, nil
}

// ImportFromFile choisit un fichier (.notecore, .zip, .md) et l'importe.
// Rien n'est jamais écrasé : voir share.Import.
func (n *NoteCore) ImportFromFile() (ImportResult, error) {
	path, err := wailsruntime.OpenFileDialog(n.callContext(), wailsruntime.OpenDialogOptions{
		Title: "Importer des notes",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Notes (*.notecore, *.md, *.zip, *.txt)", Pattern: "*.notecore;*.md;*.markdown;*.zip;*.txt"},
		},
	})
	if err != nil {
		return ImportResult{}, fmt.Errorf("dialog d'import : %w", err)
	}
	if path == "" {
		return ImportResult{Cancelled: true}, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return ImportResult{}, fmt.Errorf("lecture de %s : %w", filepath.Base(path), err)
	}
	if info.Size() > share.MaxPackageBytes {
		return ImportResult{}, fmt.Errorf("fichier trop volumineux (maximum %d Mio)", share.MaxPackageBytes>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ImportResult{}, fmt.Errorf("lecture de %s : %w", filepath.Base(path), err)
	}
	return n.runImport(data, "", "")
}

// ImportFromURL reçoit un envoi d'un autre appareil du réseau local.
func (n *NoteCore) ImportFromURL(rawURL string) (ImportResult, error) {
	data, err := share.Fetch(n.callContext(), rawURL)
	if err != nil {
		return ImportResult{}, err
	}
	return n.runImport(data, "", "")
}

// ImportPending reprend un import en attente de son mot de passe.
func (n *NoteCore) ImportPending(id, passphrase string) (ImportResult, error) {
	n.mu.Lock()
	p := n.pending
	n.mu.Unlock()
	if p == nil || p.id != id {
		return ImportResult{}, errors.New("cet import n'est plus en attente : recommencez")
	}
	return n.runImport(p.data, passphrase, p.id)
}

// CancelPending abandonne un import en attente et libère la mémoire.
func (n *NoteCore) CancelPending() {
	n.mu.Lock()
	n.pending = nil
	n.mu.Unlock()
}

// Shutdown ferme l'envoi en cours et supprime les copies temporaires.
func (n *NoteCore) Shutdown() {
	n.shares.Stop()
	n.mu.Lock()
	dirs := n.tempDirs
	n.tempDirs = nil
	n.pending = nil
	n.mu.Unlock()
	for _, d := range dirs {
		_ = os.RemoveAll(d)
	}
}

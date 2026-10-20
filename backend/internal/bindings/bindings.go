// Package bindings expose le service métier à Wails. Ils restent
// minces (ARCHITECTURE.md §2) : validation des entrées de transport,
// appel du service applicatif, conversion des erreurs en erreurs
// lisibles par l'interface.
//
// Wails v2 n'injecte pas de contexte par appel : la méthode Startup
// est appelée par le runtime avec le contexte applicatif, que le
// service réutilise ensuite (doc Wails v2, hooks de cycle de vie).
package bindings

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"

	"notecore/backend/internal/notes"
	"notecore/backend/internal/preferences"
	"notecore/backend/internal/review"
	"notecore/backend/internal/share"
)

// NoteCore est la facade unique exposée à l'interface.
type NoteCore struct {
	svc   *notes.Service
	prefs *preferences.Store
	rev   *review.Service
	db    *sql.DB
	ctx   context.Context
	// Copie de la base conservée avant une migration de schéma : annoncée
	// à l'interface, jamais un fichier silencieux sur le disque.
	startupReport string
	// Partage hors connexion (ADR-008) : un seul envoi à la fois, un seul import
	// en attente de mot de passe, et les copies temporaires à supprimer.
	shares   *share.Manager
	mu       sync.Mutex
	pending  *pendingImport
	tempDirs []string
	// Dossier des données de l'utilisateur, montré dans le guide de prise en main.
	dataDir string
}

func New(svc *notes.Service, db *sql.DB) *NoteCore {
	if db == nil {
		panic("bindings : base absente")
	}
	return &NoteCore{svc: svc, prefs: preferences.New(db), rev: review.New(db), db: db, shares: share.NewManager()}
}

// Startup reçoit le contexte du runtime Wails (appelé automatiquement).
func (n *NoteCore) Startup(ctx context.Context) {
	if ctx == nil {
		panic("startup sans contexte")
	}
	n.ctx = ctx
}

// SetStartupReport conserve le chemin de la copie de la base écrite avant
// une migration de schéma, pour que l'application puisse l'annoncer.
func (n *NoteCore) SetStartupReport(safetyCopy string) {
	n.startupReport = safetyCopy
}

// SetDataDir mémorise le dossier où vivent les notes, pour le guide.
func (n *NoteCore) SetDataDir(dir string) {
	n.dataDir = dir
}

// AppInfo décrit où l'application range les données de l'utilisateur et sur
// quel système elle tourne : le guide de prise en main l'affiche, pour que
// chacun sache quoi sauvegarder et où chercher.
type AppInfo struct {
	OS           string `json:"os"` // « linux », « windows », « darwin »
	DataDir      string `json:"dataDir"`
	DatabasePath string `json:"databasePath"`
}

func (n *NoteCore) AppInfo() (AppInfo, error) {
	return AppInfo{
		OS:           runtime.GOOS,
		DataDir:      n.dataDir,
		DatabasePath: filepath.Join(n.dataDir, "notecore.db"),
	}, nil
}

// StartupReport renvoie le chemin de la base conservée avant la migration
// appliquée à ce lancement. Chaîne vide : rien n'a bougé, les données de
// la version précédente sont directement celles en service.
func (n *NoteCore) StartupReport() (string, error) {
	return n.startupReport, nil
}

func (n *NoteCore) callContext() context.Context {
	if n.ctx == nil {
		// Ne jamais passer context.Background() aux appels runtime Wails :
		// ils exigent le contexte spécifique fourni par les hooks de cycle
		// de vie. L'échec est explicite plutôt que silencieusement rejeté.
		panic("bindings : contexte runtime Wails non initialisé (Startup non appelé)")
	}
	return n.ctx
}

// NotePatch est le patch de transport : champs optionnels, en
// camelCase JSON pour l'interface.
type NotePatch struct {
	Title    *string   `json:"title,omitempty"`
	Content  *string   `json:"content,omitempty"`
	FolderID *string   `json:"folderId,omitempty"`
	IsPinned *bool     `json:"isPinned,omitempty"`
	Tags     *[]string `json:"tags,omitempty"`
}

func makePatch(j NotePatch) notes.Patch {
	return notes.Patch{
		Title:    j.Title,
		Content:  j.Content,
		FolderID: j.FolderID,
		IsPinned: j.IsPinned,
		Tags:     j.Tags,
	}
}

// --- notes ----------------------------------------------------------

func (n *NoteCore) ListNotes() ([]notes.Note, error) {
	return n.svc.ListNotes(n.callContext())
}

func (n *NoteCore) CreateNote(title, folderID string) (notes.Note, error) {
	return n.svc.CreateNote(n.callContext(), title, folderID)
}

func (n *NoteCore) UpdateNote(id string, patch NotePatch) (notes.Note, error) {
	if id == "" {
		return notes.Note{}, fmt.Errorf("note à mettre à jour non identifiée")
	}
	return n.svc.UpdateNote(n.callContext(), id, makePatch(patch))
}

func (n *NoteCore) DeleteNote(id string) error {
	if id == "" {
		return fmt.Errorf("note à supprimer non identifiée")
	}
	return n.svc.DeleteNote(n.callContext(), id)
}

func (n *NoteCore) SearchNotes(query string) ([]notes.SearchResult, error) {
	return n.svc.SearchNotes(n.callContext(), query)
}

// --- dossiers --------------------------------------------------------

func (n *NoteCore) ListFolders() ([]notes.Folder, error) {
	return n.svc.ListFolders(n.callContext())
}

// CreateFolder crée un dossier dans parentID ("" = racine).
func (n *NoteCore) CreateFolder(name, parentID string) (notes.Folder, error) {
	return n.svc.CreateFolderIn(n.callContext(), name, parentID)
}

// MoveFolder déplace un dossier (avec tout ce qu'il contient) sous un autre
// dossier ("" = racine). Refusé s'il se retrouverait dans l'un de ses descendants.
func (n *NoteCore) MoveFolder(id, newParentID string) error {
	if id == "" {
		return fmt.Errorf("dossier à déplacer non identifié")
	}
	return n.svc.MoveFolder(n.callContext(), id, newParentID)
}

// MoveNotes range plusieurs notes dans un dossier ("" = sans dossier), tout ou rien.
func (n *NoteCore) MoveNotes(noteIDs []string, folderID string) error {
	return n.svc.MoveNotes(n.callContext(), noteIDs, folderID)
}

// SuggestFolders propose des dossiers pour une note, d'après ce qui est déjà
// rangé (vocabulaire et étiquettes). Une estimation, jamais un rangement automatique.
func (n *NoteCore) SuggestFolders(noteID string) ([]notes.Suggestion, error) {
	if noteID == "" {
		return nil, fmt.Errorf("note non identifiée")
	}
	out, err := n.svc.SuggestFolders(n.callContext(), noteID, 3)
	if out == nil {
		out = []notes.Suggestion{}
	}
	return out, err
}

// Inbox liste les notes sans dossier avec leurs meilleures propositions.
func (n *NoteCore) Inbox() ([]notes.InboxItem, error) {
	return n.svc.Inbox(n.callContext())
}

func (n *NoteCore) RenameFolder(id, name string) error {
	if id == "" {
		return fmt.Errorf("dossier à renommer non identifié")
	}
	return n.svc.RenameFolder(n.callContext(), id, name)
}

// DeleteFolder renvoie les notes déplacées à la racine — l'interface
// les affiche sans perte silencieuse (cahier des charges §3.2).
func (n *NoteCore) DeleteFolder(id string) ([]notes.Note, error) {
	if id == "" {
		return nil, fmt.Errorf("dossier à supprimer non identifié")
	}
	return n.svc.DeleteFolder(n.callContext(), id)
}

// --- préférences (cahier des charges §3.6) ---------------------------------------

// GetSettings renvoie tous les réglages locaux (table `settings`).
func (n *NoteCore) GetSettings() (map[string]string, error) {
	return n.prefs.All(n.callContext())
}

// SetSetting enregistre un réglage local. La validation des bornes est
// faite par le paquet preferences.
func (n *NoteCore) SetSetting(key, value string) error {
	return n.prefs.Set(n.callContext(), key, value)
}

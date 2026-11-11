package bindings

import (
	"context"
	"fmt"
	"os"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Export Markdown unidirectionnel : NoteCore reste la source de vérité,
// le dossier ne reçoit que des .md (jamais relus). La synchronisation est
// débouncée — une frappe en autosave ne réécrit pas des centaines de
// fichiers à chaque touche.
const (
	markdownExportDirSetting = "markdownExportDir"
	exportSyncDelay          = 2 * time.Second
)

// MarkdownExportDir renvoie le dossier configuré ("" = désactivé).
func (n *NoteCore) MarkdownExportDir() (string, error) {
	return n.prefs.Get(n.callContext(), markdownExportDirSetting)
}

// ChooseMarkdownExportDir ouvre le dialogue système, mémorise le choix
// et lance une première synchronisation. "" = annulé.
func (n *NoteCore) ChooseMarkdownExportDir() (string, error) {
	dir, err := wailsruntime.OpenDirectoryDialog(n.callContext(), wailsruntime.OpenDialogOptions{
		Title: "Exporter les notes en Markdown",
	})
	if err != nil {
		return "", fmt.Errorf("dialog de dossier : %w", err)
	}
	if dir == "" {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("création du dossier : %w", err)
	}
	if err := n.prefs.Set(n.callContext(), markdownExportDirSetting, dir); err != nil {
		return "", err
	}
	n.scheduleExportSync()
	return dir, nil
}

// SetMarkdownExportDir fixe (« » = désactiver) ou efface le dossier
// d'export. Les fichiers déjà produits restent : ils appartiennent
// désormais à l'utilisateur.
func (n *NoteCore) SetMarkdownExportDir(dir string) error {
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("création du dossier : %w", err)
		}
	}
	if err := n.prefs.Set(n.callContext(), markdownExportDirSetting, dir); err != nil {
		return err
	}
	n.scheduleExportSync()
	return nil
}

// scheduleExportSync arme une synchronisation différée : les écritures
// rapprochées (autosave, déplacements en série) fusionnent en une passe.
func (n *NoteCore) scheduleExportSync() {
	n.exportMu.Lock()
	defer n.exportMu.Unlock()
	if n.exportTimer != nil {
		n.exportTimer.Stop()
	}
	n.exportTimer = time.AfterFunc(exportSyncDelay, n.syncExportNow)
}

// syncExportNow lit le réglage et synchronise. Contexte propre : ce
// travail survit à l'appel qui l'a déclenché et ne dépend pas du
// contexte runtime Wails.
func (n *NoteCore) syncExportNow() {
	dir, err := n.prefs.Get(context.Background(), markdownExportDirSetting)
	if err != nil || dir == "" {
		return
	}
	if _, err := n.exporter.Sync(context.Background(), dir); err != nil {
		// L'export est un miroir : un échec ne doit jamais remonter dans
		// l'édition des notes. La passe suivante reprendra l'état final.
		_ = err
	}
}

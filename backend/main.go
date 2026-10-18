// main.go — point d'entrée NoteCore V1, phase 2 (cahier des charges §5).
// Monolithe modulaire : bindings Wails mince devant le service métier ;
// SQLite est la source de vérité (ADR-002/005).
package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	wails "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"

	"notecore/backend/internal/bindings"
	"notecore/backend/internal/notes"
	"notecore/backend/internal/storage"
)

//go:embed frontenddist
var dist embed.FS

//go:embed assets/notecore.png
var appIcon []byte

// userDataDirectory place la base dans le répertoire de données de
// l'utilisateur — jamais dans le répertoire d'installation
// (ARCHITECTURE.md §3).
func userDataDirectory() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("répertoire utilisateur : %w", err)
	}
	return filepath.Join(base, "NoteCore"), nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dir, err := userDataDirectory()
	if err != nil {
		return err
	}

	db, report, err := storage.OpenWithReport(filepath.Join(dir, "notecore.db"))
	if err != nil {
		return fmt.Errorf("ouverture de la base : %w", err)
	}
	defer db.Close()

	// Une installation plus récente migre le schéma : la base telle
	// qu'elle était chez l'utilisateur précédemment est conservée à côté,
	// et le chemin en est annoncé dans l'application (aucun perdu, §11).
	if report.SafetyCopy != "" {
		log.Printf(
			"migration du schéma (versions %v) : base précédente conservée dans %s",
			report.Applied, report.SafetyCopy,
		)
	}

	service := notes.New(db)
	api := bindings.New(service, db)
	api.SetStartupReport(report.SafetyCopy)

	return wails.Run(&options.App{
		Title:  "NoteCore",
		Width:  1024,
		Height: 720,
		// Seuil d'éditabilité : en dessous de 900 px la mise en page
		// replie l'éditeur (Workbench.svelte). Interdire cette zone
		// garantit qu'une fenêtre atteignable reste toujours éditable
		// (cahier des charges §8, critère 11).
		MinWidth:  940,
		MinHeight: 560,
		OnStartup: api.Startup,
		Linux: &linux.Options{
			Icon:        appIcon,
			ProgramName: "notecore",
		},
		AssetServer: &assetserver.Options{Assets: dist},
		Bind:        []interface{}{api},
	})
}

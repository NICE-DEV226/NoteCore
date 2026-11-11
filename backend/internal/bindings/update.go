package bindings

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Vérification de mise à jour : un seul appel HTTPS à la sortie du
// dernier release GitHub, à la demande (démarrage de l'app et bouton
// manuel). Jamais bloquant — hors ligne ou API injoignable, l'appel
// échoue et l'interface reste silencieuse. NoteCore ne télécharge ni
// n'installe rien : elle propose seulement le lien de la version.
const updateAPI = "https://api.github.com/repos/NICE-DEV226/NoteCore/releases/latest"

// appVersion est fixée au démarrage depuis info.productVersion de
// wails.json (main.go) : la version comparée suit toujours le binaire,
// jamais une constante à maintenir à la main.
var appVersion string

// SetAppVersion enregistre la version de l'application en cours.
func SetAppVersion(v string) { appVersion = v }

// UpdateInfo décrit ce que l'interface affiche.
type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	URL       string `json:"url"`
	Name      string `json:"name"`
}

// ghRelease est la partie de la réponse GitHub utile à l'application :
// version, page de présentation et liste des binaires téléchargeables.
type ghRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Name    string `json:"name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// fetchRelease interroge la dernière release publiée. Le contexte est
// fourni par l'appelant (timeout court pour la vérification, plus long
// pour le téléchargement qui suit).
func fetchRelease(ctx context.Context) (ghRelease, error) {
	var rel ghRelease
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateAPI, nil)
	if err != nil {
		return rel, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "NoteCore")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return rel, fmt.Errorf("vérification de mise à jour : %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return rel, fmt.Errorf("vérification de mise à jour : HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return rel, fmt.Errorf("vérification de mise à jour : réponse illisible")
	}
	return rel, nil
}

// CheckForUpdate interroge GitHub pour la dernière version publiée.
// Une erreur signifie « pas d'information » (hors ligne, délai dépassé,
// API muette) : l'appelant n'a rien à signaler.
func (n *NoteCore) CheckForUpdate() (UpdateInfo, error) {
	info := UpdateInfo{Current: appVersion}
	if appVersion == "" {
		// Version inconnue (bindings utilisés hors wails.Run) : on ne
		// prétend pas savoir s'il y a une mise à jour.
		return info, nil
	}

	ctx, cancel := context.WithTimeout(n.callContext(), 6*time.Second)
	defer cancel()
	rel, err := fetchRelease(ctx)
	if err != nil {
		return info, err
	}

	info.Latest = strings.TrimPrefix(rel.TagName, "v")
	info.URL = rel.HTMLURL
	info.Name = rel.Name
	info.Available = compareVersions(info.Latest, appVersion) > 0
	return info, nil
}

// compareVersions compare deux versions « Majeur.Mineur.Patch » :
// positif si a > b. Les pré-release et métadonnées sont ignorées.
func compareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] - pb[i]
		}
	}
	return 0
}

func versionParts(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.SplitN(v, ".", 4)
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n, _ := strconv.Atoi(strings.TrimSpace(parts[i]))
		out[i] = n
	}
	return out
}

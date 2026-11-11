// Package exporter maintient un dossier Markdown miroir des notes :
// chaque note devient un .md (avec les dossiers NoteCore comme
// sous-dossiers), dans UN SEUL SENS — NoteCore est la source de vérité,
// les fichiers produits ne sont jamais relus ni écrasés par l'utilisateur.
//
// Sureté : seuls les fichiers listés dans le manifeste
// (.notecore-export.json) sont jamais supprimés — un .md étranger dans le
// dossier n'est pas touché. Les écritures sont atomiques (fichier
// temporaire + renommage) et un contenu inchangé n'est pas réécrit, pour
// que git et les outils d'indexation ne voient pas de bruit.
package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"notecore/backend/internal/notes"
)

// ManifestName est le fichier témoin posé à la racine du dossier exporté :
// il liste les chemins que NoteCore a écrits, pour savoir lesquels il
// peut effacer lors d'une synchronisation suivante.
const ManifestName = ".notecore-export.json"

const (
	maxNameLen  = 80 // runes, marge sous les limites des systèmes de fichiers
	manifestVer = 1
)

// Result résume une synchronisation pour l'interface.
type Result struct {
	Dir    string `json:"dir"`
	Files  int    `json:"files"`
	Synced bool   `json:"synced"` // false quand le dossier n'est pas configuré
}

// Service synchronise la base vers un dossier, à la demande (debounce
// côté bindings). Une seule synchronisation à la fois.
type Service struct {
	svc *notes.Service
	sem chan struct{}
}

func New(svc *notes.Service) *Service {
	return &Service{svc: svc, sem: make(chan struct{}, 1)}
}

// Sync réécrit le miroir complet du dossier : fichiers manquants ou
// modifiés, fichiers devenus inutiles retirés (ceux du manifeste
// précédent seulement). Sans dossier configuré, c'est un no-op.
func (s *Service) Sync(ctx context.Context, dir string) (Result, error) {
	res := Result{Dir: dir}
	if strings.TrimSpace(dir) == "" {
		return res, nil
	}
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	default:
		// Une synchro tourne déjà : la suivante repartira du même état
		// final, inutile d'empiler.
		return Result{Dir: dir, Synced: true}, nil
	}

	all, err := s.svc.ListNotes(ctx)
	if err != nil {
		return res, fmt.Errorf("export : lecture des notes : %w", err)
	}
	folders, err := s.svc.ListFolders(ctx)
	if err != nil {
		return res, fmt.Errorf("export : lecture des dossiers : %w", err)
	}

	targets, dirs, err := plan(all, folders)
	if err != nil {
		return res, err
	}

	oldManifest, err := readManifest(dir)
	if err != nil {
		return res, err
	}

	for _, rel := range oldManifest.Files {
		if _, keep := targets[rel]; !keep {
			// Le fichier n'est plus produit : il figurait dans notre
			// manifeste, donc il est bien à nous — on peut l'effacer.
			_ = os.Remove(filepath.Join(dir, filepath.FromSlash(rel)))
		}
	}
	for _, d := range oldManifest.Dirs {
		if _, keep := dirs[d]; !keep {
			// Suppression des dossiers vides les plus profonds d'abord ;
			// os.Remove échoue sur un dossier non vide — jamais destructif.
			_ = os.Remove(filepath.Join(dir, filepath.FromSlash(d)))
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, fmt.Errorf("export : création du dossier : %w", err)
	}
	for d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(d)), 0o755); err != nil {
			return res, fmt.Errorf("export : création de %s : %w", d, err)
		}
	}

	written := 0
	for rel, body := range targets {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if sameFile(path, body) {
			continue
		}
		if err := writeAtomic(path, body); err != nil {
			return res, fmt.Errorf("export de %s : %w", rel, err)
		}
		written++
	}

	// Le manifeste est réécrit en dernier : s'il est à jour, la passe
	// suivante saura exactement ce qui lui appartient.
	m := manifest{Version: manifestVer, Files: sortedKeys(targets), Dirs: sortedDirKeys(dirs)}
	if err := writeAtomic(filepath.Join(dir, ManifestName), string(mustJSON(m))); err != nil {
		return res, fmt.Errorf("export : manifeste : %w", err)
	}

	res.Files = len(targets)
	res.Synced = true
	return res, nil
}

// --- planification -----------------------------------------------------------

// plan calcule les chemins cibles (relatifs, séparateur « / ») et l'ensemble
// des dossiers nécessaires. Les noms de fichiers et de dossiers sont
// désambiguïsés de façon déterministe (tri titre puis identifiant) pour que
// deux synchronisations successives ne renomment rien sans raison.
func plan(all []notes.Note, folders []notes.Folder) (map[string]string, map[string]bool, error) {
	byID := make(map[string]notes.Folder, len(folders))
	for _, f := range folders {
		byID[f.ID] = f
	}
	// Chemin de dossiers pour chaque dossier : []string de segments déjà
	// sanitizés, nil pour la racine.
	folderPath := make(map[string][]string, len(folders))
	var resolve func(id string, visiting map[string]bool) ([]string, bool)
	resolve = func(id string, visiting map[string]bool) ([]string, bool) {
		if id == "" {
			return nil, true
		}
		if visiting[id] {
			return nil, false // cycle de dossiers : jamais produit par l'UI
		}
		if p, ok := folderPath[id]; ok {
			return p, true
		}
		f, ok := byID[id]
		if !ok {
			return nil, true // dossier disparu : note à la racine
		}
		visiting[id] = true
		defer delete(visiting, id)
		parent, ok := resolve(strPtr(f.ParentID), visiting)
		if !ok {
			return nil, false
		}
		p := append(append([]string{}, parent...), sanitize(f.Name))
		folderPath[id] = p
		return p, true
	}

	dirs := map[string]bool{".": true}
	type item struct {
		relDir string
		base   string
		id     string
		title  string
		body   string
	}
	items := make([]item, 0, len(all))
	for _, n := range all {
		if n.DeletedAt != nil {
			continue // la corbeille n'est pas exportée
		}
		segs, ok := resolve(strPtr(n.FolderID), map[string]bool{})
		if !ok {
			return nil, nil, fmt.Errorf("export : arborescence de dossiers invalide")
		}
		for i := range segs {
			dirs[strings.Join(segs[:i+1], "/")] = true
		}
		relDir := strings.Join(segs, "/")
		items = append(items, item{relDir: relDir, base: sanitize(n.Title), id: n.ID, title: n.Title, body: render(n)})
	}

	// Dédoublonnage déterministe par dossier : tri (titre, id), le premier
	// garde le nom nu, les suivants reçoivent -2, -3…
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].relDir != items[j].relDir {
			return items[i].relDir < items[j].relDir
		}
		li, lj := strings.ToLower(items[i].title), strings.ToLower(items[j].title)
		if li != lj {
			return li < lj
		}
		return items[i].id < items[j].id
	})

	targets := make(map[string]string, len(items))
	counts := map[string]int{}
	for _, it := range items {
		key := it.relDir + "/" + it.base
		counts[key]++
		name := it.base
		if counts[key] > 1 {
			name = fmt.Sprintf("%s-%d", it.base, counts[key])
		}
		rel := name + ".md"
		if it.relDir != "" {
			rel = it.relDir + "/" + rel
		}
		targets[rel] = it.body
	}
	return targets, dirs, nil
}

// render produit le fichier : front-matter YAML minimal (relu par
// ParseMarkdown à l'import) puis le contenu brut de la note.
func render(n notes.Note) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + yamlQuote(n.Title) + "\n")
	if len(n.Tags) > 0 {
		b.WriteString("tags: [")
		for i, t := range n.Tags {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(yamlQuote(t))
		}
		b.WriteString("]\n")
	}
	b.WriteString("---\n")
	b.WriteString(strings.TrimRight(n.Content, "\n"))
	b.WriteString("\n")
	return b.String()
}

// yamlQuote produit une chaîne entre guillemets compatible avec
// l'unquote/splitList de share.ParseMarkdown : « \ » et « " » échappés.
func yamlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

var unsafeName = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]`)

// sanitize borne un nom de fichier ou de dossier : séparateurs et
// caractères interdits retirés, bords nettoyés, longueur plafonnée.
// « . » et « .. » sont refusés (un nom qui ne devient rien après
// nettoyage donne « sans-titre » / « sans-dossier »).
func sanitize(name string) string {
	s := unsafeName.ReplaceAllString(name, "-")
	s = strings.Trim(s, " .")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "sans-titre"
	}
	r := []rune(s)
	if len(r) > maxNameLen {
		s = strings.TrimRight(string(r[:maxNameLen]), " .-")
		if s == "" {
			s = "sans-titre"
		}
	}
	return s
}

func strPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// --- fichiers ----------------------------------------------------------------

func sameFile(path string, body string) bool {
	existing, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return string(existing) == body
}

// writeAtomic écrit via un temporaire dans le même dossier puis renomme :
// un lecteur ne voit jamais un fichier à moitié écrit.
func writeAtomic(path string, body string) error {
	tmp := path + ".tmp-notecore"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

type manifest struct {
	Version int      `json:"version"`
	Files   []string `json:"files"`
	Dirs    []string `json:"dirs"`
}

func readManifest(dir string) (manifest, error) {
	var m manifest
	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return m, fmt.Errorf("export : manifeste illisible : %w", err)
	}
	if err := json.Unmarshal(raw, &m); err != nil || m.Version != manifestVer {
		// Manifeste corrompu ou d'une autre version : on ne touche à rien —
		// mieux vaut laisser des fichiers en trop qu'en effacer à tort.
		return manifest{}, nil
	}
	return m, nil
}

func mustJSON(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return append(b, '\n')
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedDirKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

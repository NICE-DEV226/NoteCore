// Rangement intelligent (ADR-009) : proposer le dossier d'une note d'après ce
// qui est DÉJÀ rangé. Aucun modèle, aucun réseau : une comparaison de
// vocabulaire (TF-IDF, similarité cosinus) et d'étiquettes, calculée sur la base
// locale. C'est une estimation, jamais un rangement automatique : l'utilisateur
// accepte ou non chaque proposition.
package notes

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
)

// Poids des signaux dans le titre et les étiquettes, par rapport au texte.
const (
	titleWeight = 3.0
	tagWeight   = 4.0
	// descendantWeight : les notes des sous-dossiers comptent pour le dossier
	// parent, moins que ses propres notes.
	descendantWeight = 0.4
	maxScanChars     = 20000
	minSuggestScore  = 0.08
	minTokens        = 3
)

// Suggestion est un dossier proposé pour une note.
type Suggestion struct {
	FolderID string   `json:"folderId"`
	Path     []string `json:"path"` // de la racine au dossier
	Score    float64  `json:"score"`
	// Confidence : « high » (à proposer en un clic), « medium » ou « low ».
	Confidence string   `json:"confidence"`
	Tags       []string `json:"tags"`  // étiquettes que la note partage avec le dossier
	Terms      []string `json:"terms"` // mots qui motivent la proposition
}

// InboxItem : une note sans dossier et ses meilleures propositions.
type InboxItem struct {
	NoteID      string       `json:"noteId"`
	Suggestions []Suggestion `json:"suggestions"`
}

var stopwords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`
		les des une aux ces ses mes tes nos vos leur leurs dans sur sous par pour avec sans chez vers entre
		est sont etre ete etait fait faire peut peuvent plus moins tres tout tous toute toutes aussi comme
		mais donc car que qui quoi dont dit cet cette celui celle ceux elle elles nous vous ils
		pas non oui ainsi alors encore deja puis apres avant pendant selon sera seront avoir ont
		the and for are was were with that this from not but have has had its into than then they them
		you your can will would could should about there their which what when where while also
		note notes cours exemple voir`) {
		m[w] = true
	}
	return m
}()

var accentFold = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "é", "e", "è", "e", "ê", "e", "ë", "e", "î", "i", "ï", "i",
	"ô", "o", "ö", "o", "ù", "u", "û", "u", "ü", "u", "ç", "c", "œ", "oe", "æ", "ae", "ÿ", "y",
)

// tokenize découpe un texte en mots comparables : minuscules, sans accents,
// sans mots vides, sans nombres seuls.
func tokenize(text string) []string {
	text = accentFold.Replace(strings.ToLower(text))
	var out []string
	var cur []rune
	flush := func() {
		if n := len(cur); n >= 3 && n <= 30 {
			w := string(cur)
			if !stopwords[w] && strings.IndexFunc(w, unicode.IsLetter) >= 0 {
				out = append(out, w)
			}
		}
		cur = cur[:0]
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur = append(cur, r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

type sparse map[string]float64

func (v sparse) add(o sparse, w float64) {
	for k, x := range o {
		v[k] += x * w
	}
}

func (v sparse) norm() float64 {
	var s float64
	for _, x := range v {
		s += x * x
	}
	return math.Sqrt(s)
}

func cosine(a, b sparse, na, nb float64) float64 {
	if na == 0 || nb == 0 {
		return 0
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	var dot float64
	for k, x := range a {
		dot += x * b[k]
	}
	return dot / (na * nb)
}

type model struct {
	folders  map[string]Folder
	notes    []Note
	vec      map[string]sparse   // note → vecteur TF-IDF (non normalisé)
	tagsOf   map[string][]string // note → étiquettes normalisées
	tagTotal map[string]int      // étiquette → nombre de notes
	sums     map[string]sparse   // dossier → somme pondérée des vecteurs
	tagIn    map[string]map[string]int
	tokenCnt map[string]int // note → nombre de mots utiles
	path     map[string][]string
}

func (s *Service) buildModel(ctx context.Context) (*model, error) {
	all, err := s.ListNotes(ctx)
	if err != nil {
		return nil, err
	}
	folders, err := s.ListFolders(ctx)
	if err != nil {
		return nil, err
	}
	m := &model{
		folders: make(map[string]Folder, len(folders)), notes: all,
		vec: make(map[string]sparse, len(all)), tagsOf: make(map[string][]string, len(all)),
		tagTotal: map[string]int{}, sums: map[string]sparse{}, tagIn: map[string]map[string]int{},
		tokenCnt: make(map[string]int, len(all)), path: map[string][]string{},
	}
	for _, f := range folders {
		m.folders[f.ID] = f
	}
	for _, f := range folders {
		m.path[f.ID] = m.pathOf(f.ID)
	}

	// Fréquences brutes puis IDF sur l'ensemble des notes.
	raw := make(map[string]sparse, len(all))
	df := map[string]int{}
	for _, n := range all {
		content := n.Content
		if len(content) > maxScanChars {
			content = content[:maxScanChars]
		}
		counts := map[string]float64{}
		body := tokenize(content)
		for _, w := range body {
			counts[w]++
		}
		for _, w := range tokenize(n.Title) {
			counts[w] += titleWeight
		}
		tags := make([]string, 0, len(n.Tags))
		for _, t := range n.Tags {
			t = strings.ToLower(strings.TrimSpace(accentFold.Replace(strings.ToLower(t))))
			if t == "" {
				continue
			}
			tags = append(tags, t)
			counts["#"+t] += tagWeight
			for _, w := range tokenize(t) {
				counts[w] += tagWeight / 2
			}
		}
		m.tagsOf[n.ID] = tags
		m.tokenCnt[n.ID] = len(body) + len(tags)
		v := make(sparse, len(counts))
		for w, c := range counts {
			v[w] = 1 + math.Log(c)
			df[w]++
		}
		raw[n.ID] = v
	}
	total := float64(len(all))
	for _, n := range all {
		v := raw[n.ID]
		for w := range v {
			v[w] *= math.Log((total+1)/(float64(df[w])+1)) + 1
		}
		m.vec[n.ID] = v
	}

	// Somme des vecteurs par dossier : propres notes (poids 1) et sous-dossiers (poids réduit).
	for _, n := range all {
		for _, t := range m.tagsOf[n.ID] {
			m.tagTotal[t]++
		}
		if n.FolderID == nil {
			continue
		}
		nv := m.vec[n.ID]
		nn := nv.norm()
		if nn == 0 {
			continue
		}
		for cur, depth := *n.FolderID, 0; cur != "" && depth <= MaxFolderDepth+1; depth++ {
			f, ok := m.folders[cur]
			if !ok {
				break
			}
			w := 1.0
			if depth > 0 {
				w = descendantWeight
			}
			if m.sums[cur] == nil {
				m.sums[cur] = sparse{}
				m.tagIn[cur] = map[string]int{}
			}
			m.sums[cur].add(nv, w/nn)
			for _, t := range m.tagsOf[n.ID] {
				m.tagIn[cur][t]++
			}
			if f.ParentID == nil {
				break
			}
			cur = *f.ParentID
		}
	}
	return m, nil
}

func (m *model) pathOf(id string) []string {
	var rev []string
	for cur, n := id, 0; cur != "" && n <= MaxFolderDepth+2; n++ {
		f, ok := m.folders[cur]
		if !ok {
			break
		}
		rev = append(rev, f.Name)
		if f.ParentID == nil {
			break
		}
		cur = *f.ParentID
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// suggest classe les dossiers pour la note donnée. `self` est le dossier déjà
// occupé par la note, dont on retire sa propre contribution (sinon une note
// déjà rangée se « reconnaîtrait » elle-même).
func (m *model) suggest(n Note, limit int) []Suggestion {
	if m.tokenCnt[n.ID] < minTokens {
		return nil // trop peu de matière pour comparer honnêtement
	}
	nv := m.vec[n.ID]
	nn := nv.norm()
	tags := m.tagsOf[n.ID]

	type cand struct {
		id    string
		score float64
		sug   Suggestion
	}
	var cands []cand
	for fid, sum := range m.sums {
		centroid := sum
		cn := sum.norm()
		// Retrait de la note elle-même si elle compte dans ce dossier.
		counted := n.FolderID != nil && m.inSubtree(*n.FolderID, fid)
		if counted && nn > 0 {
			w := descendantWeight
			if *n.FolderID == fid {
				w = 1
			}
			centroid = sparse{}
			centroid.add(sum, 1)
			centroid.add(nv, -w/nn)
			cn = centroid.norm()
		}
		cos := cosine(nv, centroid, nn, cn)

		var tagScore float64
		var shared []string
		for _, t := range tags {
			total := m.tagTotal[t]
			in := m.tagIn[fid][t]
			if counted {
				total--
				in--
			}
			if total <= 0 || in <= 0 {
				continue
			}
			shared = append(shared, t)
			support := math.Min(1, float64(total)/3)
			if sc := float64(in) / float64(total) * support; sc > tagScore {
				tagScore = sc
			}
		}
		score := 0.65*cos + 0.35*tagScore
		if score < minSuggestScore {
			continue
		}

		type tc struct {
			term string
			w    float64
		}
		var best []tc
		for term, x := range nv {
			if strings.HasPrefix(term, "#") {
				continue
			}
			if y := centroid[term]; y > 0 {
				best = append(best, tc{term, x * y})
			}
		}
		sort.Slice(best, func(i, j int) bool {
			return best[i].w > best[j].w || (best[i].w == best[j].w && best[i].term < best[j].term)
		})
		terms := []string{}
		for i := 0; i < len(best) && i < 3; i++ {
			terms = append(terms, best[i].term)
		}
		if len(shared) > 3 {
			shared = shared[:3]
		}
		cands = append(cands, cand{fid, score, Suggestion{
			FolderID: fid, Path: m.path[fid], Score: math.Round(score*1000) / 1000, Tags: shared, Terms: terms,
		}})
	}
	sort.Slice(cands, func(i, j int) bool {
		if math.Abs(cands[i].score-cands[j].score) > 1e-9 {
			return cands[i].score > cands[j].score
		}
		// À égalité (un dossier parent n'a que les notes de son enfant), le
		// dossier le plus précis passe avant.
		if li, lj := len(cands[i].sug.Path), len(cands[j].sug.Path); li != lj {
			return li > lj
		}
		return cands[i].id < cands[j].id
	})
	if limit > 0 && len(cands) > limit {
		cands = cands[:limit]
	}
	out := make([]Suggestion, len(cands))
	for i, c := range cands {
		sg := c.sug
		margin := c.score
		if i+1 < len(cands) {
			margin = c.score - cands[i+1].score
		}
		switch {
		case i == 0 && c.score >= 0.45 && margin >= 0.08:
			sg.Confidence = "high"
		case c.score >= 0.25:
			sg.Confidence = "medium"
		default:
			sg.Confidence = "low"
		}
		if i > 0 && sg.Confidence == "high" {
			sg.Confidence = "medium"
		}
		out[i] = sg
	}
	return out
}

// inSubtree : le dossier `folder` est `root` ou l'un de ses descendants.
func (m *model) inSubtree(folder, root string) bool {
	for cur, n := folder, 0; cur != "" && n <= MaxFolderDepth+2; n++ {
		if cur == root {
			return true
		}
		f, ok := m.folders[cur]
		if !ok || f.ParentID == nil {
			return false
		}
		cur = *f.ParentID
	}
	return false
}

// SuggestFolders propose des dossiers pour une note (les plus probables d'abord).
func (s *Service) SuggestFolders(ctx context.Context, noteID string, limit int) ([]Suggestion, error) {
	m, err := s.buildModel(ctx)
	if err != nil {
		return nil, err
	}
	for _, n := range m.notes {
		if n.ID == noteID {
			return m.suggest(n, limit), nil
		}
	}
	return nil, errNoteNotFound(noteID)
}

// Inbox liste les notes sans dossier, avec leurs deux meilleures propositions.
// Les notes les plus faciles à ranger (confiance haute) viennent en premier.
func (s *Service) Inbox(ctx context.Context) ([]InboxItem, error) {
	m, err := s.buildModel(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]InboxItem, 0)
	for _, n := range m.notes {
		if n.FolderID != nil {
			continue
		}
		out = append(out, InboxItem{NoteID: n.ID, Suggestions: m.suggest(n, 2)})
	}
	rank := func(it InboxItem) float64 {
		if len(it.Suggestions) == 0 {
			return -1
		}
		return it.Suggestions[0].Score
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) > rank(out[j]) })
	return out, nil
}

func errNoteNotFound(id string) error { return fmt.Errorf("note %q introuvable", id) }

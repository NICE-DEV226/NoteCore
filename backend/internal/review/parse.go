// Package review porte la révision espacée (CDC V2, ADR-007) : les cartes
// sont écrites DANS les notes, en Markdown, puis dérivées par ce paquet.
// Il n'existe donc jamais deux copies d'une même connaissance : la note est
// l'unique source du texte, SQLite ne conserve que l'état d'apprentissage.
package review

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Kind distingue les deux syntaxes de carte.
type Kind string

const (
	// KindCloze : « {{c1::réponse}} » ou « {{c1::réponse::indice}} ».
	KindCloze Kind = "cloze"
	// KindQA : une ligne « Question :: Réponse ».
	KindQA Kind = "qa"
)

// Draft est une carte telle que le texte d'une note la décrit : aucun état
// d'apprentissage, uniquement ce qui se lit.
type Draft struct {
	// Key identifie la carte dans sa note, de façon stable : « c1 » pour un
	// trou (le numéro est écrit par l'utilisateur, modifier le texte ne perd
	// donc pas la progression) ; « q:<empreinte> » pour une question.
	Key   string
	Kind  Kind
	Front string
	Back  string
}

const (
	maxClozeNumber = 99
	maxDrafts      = 500 // une note n'est pas un paquet : borne anti-abus
	hiddenMarker   = "[…]"
)

var (
	inlineCodeRe = regexp.MustCompile("`[^`\n]*`")
	fenceRe      = regexp.MustCompile("^\\s{0,3}```")
	listMarkerRe = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+(?:\[[ xX]\]\s+)?`)
	quoteRe      = regexp.MustCompile(`^\s*>\s?`)
	qaRe         = regexp.MustCompile(`^(.*?\S)\s::\s(\S.*)$`)
)

// cloze est un trou repéré dans une ligne.
type cloze struct {
	start, end int // bornes de « {{ … }} » dans la ligne
	n          int
	answer     string
	hint       string
}

// Parse extrait les cartes d'un contenu Markdown. Les blocs de code et le
// code en ligne sont ignorés : une accolade dans un exemple de code n'est pas
// une carte. L'ordre de sortie est celui du texte.
func Parse(content string) []Draft {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	type group struct {
		lines []string
		spans [][]cloze // trous de chaque ligne retenue
	}
	groups := map[int]*group{}
	var order []int
	var qas []Draft
	seenQA := map[string]bool{}

	inFence := false
	for _, line := range lines {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		masked := maskCode(line)
		spans := findClozes(line, masked)
		if len(spans) > 0 {
			seen := map[int]bool{}
			for _, sp := range spans {
				if seen[sp.n] {
					continue
				}
				seen[sp.n] = true
				g := groups[sp.n]
				if g == nil {
					g = &group{}
					groups[sp.n] = g
					order = append(order, sp.n)
				}
				g.lines = append(g.lines, line)
				g.spans = append(g.spans, spans)
			}
			continue
		}
		if d, ok := parseQA(line, masked); ok && !seenQA[d.Key] {
			seenQA[d.Key] = true
			qas = append(qas, d)
		}
	}

	sort.Ints(order)
	out := make([]Draft, 0, len(order)+len(qas))
	for _, n := range order {
		g := groups[n]
		var front, back []string
		for i, line := range g.lines {
			front = append(front, render(line, g.spans[i], n, true))
			back = append(back, render(line, g.spans[i], n, false))
		}
		out = append(out, Draft{
			Key:   "c" + strconv.Itoa(n),
			Kind:  KindCloze,
			Front: strings.Join(front, "\n"),
			Back:  strings.Join(back, "\n"),
		})
	}
	out = append(out, qas...)
	if len(out) > maxDrafts {
		out = out[:maxDrafts]
	}
	return out
}

// maskCode remplace le code en ligne par des espaces, sans changer les
// positions : les recherches se font sur le masque, le rendu sur l'original.
func maskCode(line string) string {
	return inlineCodeRe.ReplaceAllStringFunc(line, func(m string) string {
		return strings.Repeat(" ", len(m))
	})
}

// findClozes repère les « {{cN::…}} » d'une ligne. Une accolade fermante
// manquante ou un numéro invalide laissent le texte tel quel : une note en
// cours de frappe ne produit jamais d'erreur.
func findClozes(line, masked string) []cloze {
	var out []cloze
	i := 0
	for {
		open := strings.Index(masked[i:], "{{c")
		if open < 0 {
			return out
		}
		open += i
		j := open + 3
		for j < len(masked) && masked[j] >= '0' && masked[j] <= '9' {
			j++
		}
		digits := masked[open+3 : j]
		if digits == "" || len(digits) > 2 || !strings.HasPrefix(masked[j:], "::") {
			i = open + 3
			continue
		}
		n, _ := strconv.Atoi(digits)
		bodyStart := j + 2
		closeAt := strings.Index(masked[bodyStart:], "}}")
		if closeAt < 0 || n < 1 || n > maxClozeNumber {
			i = open + 3
			continue
		}
		body := line[bodyStart : bodyStart+closeAt]
		answer, hint := body, ""
		if k := strings.Index(masked[bodyStart:bodyStart+closeAt], "::"); k >= 0 {
			answer, hint = body[:k], body[k+2:]
		}
		answer = strings.TrimSpace(answer)
		if answer == "" {
			i = bodyStart + closeAt + 2
			continue
		}
		end := bodyStart + closeAt + 2
		out = append(out, cloze{start: open, end: end, n: n, answer: answer, hint: strings.TrimSpace(hint)})
		i = end
	}
}

// render reconstitue la ligne : le trou numéro n est masqué au recto et mis
// en évidence au verso ; les autres trous restent lisibles comme du texte.
func render(line string, spans []cloze, n int, front bool) string {
	var b strings.Builder
	prev := 0
	for _, sp := range spans {
		b.WriteString(line[prev:sp.start])
		switch {
		case sp.n != n:
			b.WriteString(sp.answer)
		case front && sp.hint != "":
			b.WriteString("[" + sp.hint + "]")
		case front:
			b.WriteString(hiddenMarker)
		default:
			b.WriteString("==" + sp.answer + "==")
		}
		prev = sp.end
	}
	b.WriteString(line[prev:])
	return b.String()
}

func parseQA(line, masked string) (Draft, bool) {
	text := quoteRe.ReplaceAllString(line, "")
	text = listMarkerRe.ReplaceAllString(text, "")
	if strings.Contains(maskCode(text), "{{") && strings.Contains(text, "}}") {
		return Draft{}, false
	}
	m := qaRe.FindStringSubmatch(text)
	if m == nil || strings.Contains(maskCode(m[1]), " :: ") {
		return Draft{}, false
	}
	// Un « :: » dans du code en ligne n'est pas un séparateur.
	if strings.Index(maskCode(text), " :: ") < 0 {
		return Draft{}, false
	}
	q, a := strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	if q == "" || a == "" {
		return Draft{}, false
	}
	return Draft{Key: "q:" + questionHash(q), Kind: KindQA, Front: q, Back: a}, true
}

// questionHash donne une identité stable à une question : insensible à la
// casse et aux espaces, pour qu'une retouche typographique ne crée pas de
// nouvelle carte.
func questionHash(q string) string {
	norm := strings.Join(strings.Fields(strings.ToLower(q)), " ")
	sum := sha1.Sum([]byte(norm))
	return hex.EncodeToString(sum[:])[:12]
}

// NextClozeNumber renvoie le plus petit numéro libre après les trous
// existants : l'éditeur s'en sert pour « Ajouter un trou ».
func NextClozeNumber(content string) int {
	max := 0
	for _, d := range Parse(content) {
		if d.Kind != KindCloze {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(d.Key, "c")); err == nil && n > max {
			max = n
		}
	}
	return max + 1
}

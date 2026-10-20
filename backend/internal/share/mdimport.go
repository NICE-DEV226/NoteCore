package share

import (
	"path/filepath"
	"regexp"
	"strings"
)

// bom : marque d'ordre des octets que certains éditeurs Windows écrivent en tête.
var bom = string(rune(0xFEFF))

var headingRe = regexp.MustCompile(`^#\s+(.+?)\s*#*\s*$`)

// ParseMarkdown extrait titre, étiquettes et contenu d'un fichier Markdown.
// Il comprend l'en-tête YAML que NoteCore et la plupart des outils écrivent
// (`title:`, `tags:` en liste en ligne, séparés par des virgules ou en liste à
// tirets). Sans titre explicite : le premier titre `# …`, puis le nom du
// fichier. Rien n'est jamais refusé : un fichier sans en-tête est une note.
func ParseMarkdown(raw, filename string) (title, content string, tags []string) {
	text := strings.ReplaceAll(strings.TrimPrefix(raw, bom), "\r\n", "\n")
	body := text

	if strings.HasPrefix(text, "---\n") {
		if end := strings.Index(text[4:], "\n---"); end >= 0 {
			header := text[4 : 4+end]
			rest := text[4+end+4:]
			if i := strings.IndexByte(rest, '\n'); i >= 0 {
				rest = rest[i+1:]
			} else {
				rest = ""
			}
			body = strings.TrimLeft(rest, "\n")
			title, tags = parseHeader(header)
		}
	}

	if title == "" {
		for _, line := range strings.Split(body, "\n") {
			if m := headingRe.FindStringSubmatch(line); m != nil {
				title = m[1]
				break
			}
			if strings.TrimSpace(line) != "" {
				break
			}
		}
	}
	if title == "" {
		base := filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return strings.TrimSpace(title), strings.TrimRight(body, "\n") + "\n", tags
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		inner := s[1 : len(s)-1]
		if s[0] == '"' {
			inner = strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(inner)
		}
		return inner
	}
	return s
}

// splitList découpe « [a, "b, c", d] » en respectant les guillemets.
func splitList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	var out []string
	var cur strings.Builder
	var quote rune
	flush := func() {
		if p := unquote(cur.String()); p != "" {
			out = append(out, strings.TrimPrefix(p, "#"))
		}
		cur.Reset()
	}
	for _, r := range s {
		switch {
		case quote != 0:
			cur.WriteRune(r)
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
			cur.WriteRune(r)
		case r == ',':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

func parseHeader(header string) (title string, tags []string) {
	lines := strings.Split(header, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "title":
			title = unquote(value)
		case "tags", "tag", "étiquettes":
			if strings.TrimSpace(value) != "" {
				tags = append(tags, splitList(value)...)
				continue
			}
			// liste à tirets sur les lignes suivantes
			for i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "- ") {
				i++
				if t := unquote(strings.TrimPrefix(strings.TrimSpace(lines[i]), "- ")); t != "" {
					tags = append(tags, strings.TrimPrefix(t, "#"))
				}
			}
		}
	}
	return title, tags
}

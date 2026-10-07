package gen

import (
	"fmt"
	"slices"
	"strings"

	"github.com/docuconf/docuconf-cobol/internal/copybook"
)

// tag is one @annotation with its values.
type tag struct {
	name   string // normalised: lower case, no dashes (maxlength)
	as     string // as written, for messages
	values []string
	line   int
}

// doc is what the comment block above an entry says.
type doc struct {
	text []string // description lines
	tags []tag
}

// normTag lets @max-length, @maxLength and @maxlength mean the same.
func normTag(s string) string { return strings.ToLower(strings.ReplaceAll(s, "-", "")) }

// parseDoc splits comment lines into description text and tags. A line
// starting with @ holds tags; any other line is description, except a
// line of punctuation only (a separator such as *> -----).
func parseDoc(comments []copybook.Comment) (doc, error) {
	var d doc
	for _, c := range comments {
		t := strings.TrimSpace(c.Text)
		if t == "" || strings.Trim(t, "-=*_#~+. ") == "" {
			continue
		}
		if !strings.HasPrefix(t, "@") {
			// Tags may follow text on the same line, as in an inline
			// comment: "*> port number @default 80".
			at := tagStart(t)
			if at < 0 {
				d.text = append(d.text, t)
				continue
			}
			if desc := strings.TrimSpace(t[:at]); desc != "" {
				d.text = append(d.text, desc)
			}
			t = t[at:]
		}
		words, err := splitWords(t)
		if err != nil {
			return d, fmt.Errorf("line %d: %v", c.Line, err)
		}
		for _, w := range words {
			if !w.quoted && strings.HasPrefix(w.text, "@") && len(w.text) > 1 {
				d.tags = append(d.tags, tag{name: normTag(w.text[1:]), as: w.text, line: c.Line})
				continue
			}
			if len(d.tags) == 0 {
				return d, fmt.Errorf("line %d: %q is not a tag", c.Line, w.text)
			}
			last := &d.tags[len(d.tags)-1]
			last.values = append(last.values, w.text)
		}
	}
	return d, nil
}

// tagStart is the offset of the first word in s, outside double quotes,
// that is a known tag (@ followed by a tag name), or -1. A word such as
// @home or ops@example.com in a description is not a tag.
func tagStart(s string) int {
	quoted := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '"':
			quoted = !quoted
		case !quoted && s[i] == '@' && (i == 0 || s[i-1] == ' '):
			j := i + 1
			for j < len(s) && s[j] != ' ' {
				j++
			}
			if slices.Contains(allTags, normTag(s[i+1:j])) {
				return i
			}
		}
	}
	return -1
}

// suggest returns " (did you mean @x?)" when name is within two edits of
// a tag in known, else "".
func suggest(name string, known []string) string {
	best, bestD := "", 3
	for _, k := range known {
		if d := editDistance(name, k); d < bestD {
			best, bestD = k, d
		}
	}
	if best == "" {
		return ""
	}
	return fmt.Sprintf("; did you mean @%s?", tagSpelling[best])
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// recordTags are the tags of the level-01 record.
var recordTags = []string{"service", "program", "package", "prefix"}

// allTags lists every tag, by normalised name.
var allTags = slices.Concat(recordTags, knownVarTags, knownFileTags, []string{"ignore"})

// tagSpelling is how the README spells each tag, for suggestions.
var tagSpelling = func() map[string]string {
	m := map[string]string{}
	for _, t := range []string{"service", "program", "package", "prefix", "env", "desc", "type", "secret",
		"required", "default", "min", "max", "min-length", "max-length", "pattern", "schemes", "values",
		"min-items", "max-items", "item-min", "item-max", "encoding", "separator", "unit", "count", "present",
		"examples", "deprecated", "group", "schema", "config-key", "ignore", "file", "path", "path-env",
		"reload", "max-size", "format", "dns-names", "key-algorithms", "min-remaining", "require-ca",
		"min-certificates", "password-var"} {
		m[normTag(t)] = t
	}
	return m
}()

type word struct {
	text   string
	quoted bool
}

// splitWords splits on spaces. A double-quoted word may hold spaces; a
// quote inside it is doubled, as in a COBOL literal.
func splitWords(s string) ([]word, error) {
	var out []word
	for i := 0; i < len(s); {
		switch {
		case s[i] == ' ':
			i++
		case s[i] == '"':
			var b strings.Builder
			j := i + 1
			for {
				if j >= len(s) {
					return nil, fmt.Errorf("unclosed quote in %q", s)
				}
				if s[j] == '"' {
					if j+1 < len(s) && s[j+1] == '"' {
						b.WriteByte('"')
						j += 2
						continue
					}
					break
				}
				b.WriteByte(s[j])
				j++
			}
			out = append(out, word{b.String(), true})
			i = j + 1
		default:
			j := i
			for j < len(s) && s[j] != ' ' {
				j++
			}
			out = append(out, word{s[i:j], false})
			i = j
		}
	}
	return out, nil
}

func (d doc) get(name string) (tag, bool) {
	for _, t := range d.tags {
		if t.name == name {
			return t, true
		}
	}
	return tag{}, false
}

func (d doc) has(name string) bool { _, ok := d.get(name); return ok }

func (d doc) description() string {
	if t, ok := d.get("desc"); ok {
		return strings.Join(t.values, " ")
	}
	return strings.Join(d.text, " ")
}

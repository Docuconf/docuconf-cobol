package gen

import (
	"fmt"
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
			d.text = append(d.text, t)
			continue
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

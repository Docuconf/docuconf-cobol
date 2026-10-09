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
	// lines is the comment's text without its tags, with indentation
	// kept; "" separates paragraphs. Its first paragraph is the
	// description and the rest the details (see description, details).
	lines []string
	tags  []tag
}

// normTag lets @max-length, @maxLength and @maxlength mean the same.
func normTag(s string) string { return strings.ToLower(strings.ReplaceAll(s, "-", "")) }

// parseDoc splits comment lines into text and tags. A line starting with
// @ holds tags; any other line is text, except a line of punctuation only
// (a separator such as *> -----). An empty comment line (*>) ends a
// paragraph. Inside a fenced code block (``` or ~~~), every line is text.
func parseDoc(comments []copybook.Comment) (doc, error) {
	var d doc
	fence := ""
	for _, c := range comments {
		t := strings.TrimSpace(c.Text)
		raw := c.Raw
		if raw == "" {
			raw = t
		}
		if fence != "" || strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			switch {
			case fence == "":
				fence = t[:3]
			case strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "":
				fence = ""
			}
			d.lines = append(d.lines, raw)
			continue
		}
		if t == "" {
			d.lines = append(d.lines, "")
			continue
		}
		if strings.Trim(t, "-=*_#~+. ") == "" {
			continue
		}
		if !strings.HasPrefix(t, "@") {
			// Tags may follow text on the same line, as in an inline
			// comment: "*> port number @default 80".
			at := tagStart(t)
			if at < 0 {
				d.lines = append(d.lines, raw)
				continue
			}
			if desc := strings.TrimSpace(t[:at]); desc != "" {
				d.lines = append(d.lines, desc)
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
var recordTags = []string{"service", "program", "package", "prefix", "appversion"}

// allTags lists every tag, by normalised name.
var allTags = slices.Concat(recordTags, knownVarTags, knownFileTags, []string{"ignore"})

// tagSpelling is how the README spells each tag, for suggestions.
var tagSpelling = func() map[string]string {
	m := map[string]string{}
	for _, t := range []string{"service", "program", "package", "prefix", "env", "desc", "details", "type", "secret",
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

// paragraphs returns the comment text, unindented and without blank lines
// at either end, and the index of the line that ends its first paragraph.
func (d doc) paragraphs() ([]string, int) {
	indent := -1
	for _, l := range d.lines {
		if strings.TrimSpace(l) != "" {
			if n := len(l) - len(strings.TrimLeft(l, " ")); indent < 0 || n < indent {
				indent = n
			}
		}
	}
	var lines []string
	for _, l := range d.lines {
		if strings.TrimSpace(l) == "" {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			continue
		}
		lines = append(lines, l[indent:])
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	end := slices.Index(lines, "")
	if end < 0 {
		end = len(lines)
	}
	return lines, end
}

// description is @desc, or the first paragraph of the comment on one
// line.
func (d doc) description() string {
	if t, ok := d.get("desc"); ok {
		return strings.Join(t.values, " ")
	}
	lines, end := d.paragraphs()
	var words []string
	for _, l := range lines[:end] {
		words = append(words, strings.Fields(l)...)
	}
	return strings.Join(words, " ")
}

// details is @details, or the comment after its first paragraph, as
// CommonMark (SPEC §4.2): docs only, never read at runtime. COBOL has no
// doc comment syntax of its own, so the text is used as written. ok is
// false when there are none.
func (d doc) details() (string, bool) {
	if t, ok := d.get("details"); ok {
		return strings.Join(t.values, " "), true
	}
	lines, end := d.paragraphs()
	if end >= len(lines) {
		return "", false
	}
	return strings.Join(lines[end+1:], "\n"), true
}

// maxDetails is the most characters (Unicode code points) details may
// have (SPEC §4.2).
const maxDetails = 4000

// checkDetails reports details that are blank or too long, as the
// contract's #Details does.
func checkDetails(details string, fail func(string, ...any)) {
	switch n := len([]rune(details)); {
	case strings.TrimSpace(details) == "":
		fail("@details must not be blank")
	case n > maxDetails:
		fail("details are %d characters (the comment after its first paragraph, or @details); details may have at most %d, as the docuconf spec requires (SPEC §4.2)", n, maxDetails)
	}
}

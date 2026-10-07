// Package copybook parses the data description entries of a COBOL
// copybook, with the comment lines above each entry, which carry
// docuconf's annotations.
//
// It reads the subset of COBOL a configuration record needs: level
// numbers, data names, PICTURE, USAGE, OCCURS (with DEPENDING ON), VALUE
// and level-88 condition names. Other clauses are skipped.
package copybook

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Format is the reference format of the source, as cobc -fixed / -free.
type Format int

const (
	Fixed Format = iota // columns 8-72, indicator in column 7
	Free
)

// Error is a problem at a line of the copybook.
type Error struct {
	File string
	Line int
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg) }

// Comment is one comment line, without its *> or * indicator.
type Comment struct {
	Line int
	Text string
}

// Entry is one data description entry.
type Entry struct {
	Line     int
	Level    int
	Name     string // "" or FILLER for an unnamed item
	Pic      string // as written, without PIC/PICTURE
	Usage    string // DISPLAY when not given
	Occurs   int    // 0 when the item is not a table
	OccursLo int    // the minimum of OCCURS lo TO hi
	Depends  string // DEPENDING ON item
	Values   []Literal
	Doc      []Comment // the comment block directly above, and inline comments
	Children []*Entry
	Parent   *Entry
	Redefine string
}

// Literal is a VALUE literal.
type Literal struct {
	Text    string // the value: string contents, or the number as written
	Numeric bool
	Thru    *Literal
}

// IsGroup reports whether the entry is a group item.
func (e *Entry) IsGroup() bool { return e.Pic == "" && len(e.Children) > 0 && e.Level != 88 }

// Conditions returns the level-88 children.
func (e *Entry) Conditions() []*Entry {
	var out []*Entry
	for _, c := range e.Children {
		if c.Level == 88 {
			out = append(out, c)
		}
	}
	return out
}

type token struct {
	text    string
	line    int
	literal bool // a quoted literal; text is its contents
	period  bool
}

type line struct {
	n       int
	code    string
	comment *Comment // a whole-line comment
	inline  string   // a *> comment after code
	blank   bool
}

// Parse reads the entries of a copybook. It returns the top-level
// entries (level 01 and 77), with their subordinates as children.
func Parse(file, src string, format Format) ([]*Entry, error) {
	lines, err := splitLines(file, src, format)
	if err != nil {
		return nil, err
	}
	var toks []token
	var pending []Comment // comments since the last entry ended
	docs := map[int][]Comment{}
	starts := map[int]bool{} // token index -> starts an entry
	expectStart := true
	for _, l := range lines {
		switch {
		case l.comment != nil:
			pending = append(pending, *l.comment)
			continue
		case l.blank:
			pending = nil
			continue
		}
		lt, err := tokenize(file, l)
		if err != nil {
			return nil, err
		}
		for _, t := range lt {
			if expectStart && !t.period {
				starts[len(toks)] = true
				docs[len(toks)] = pending
				pending = nil
				expectStart = false
			}
			toks = append(toks, t)
			if t.period {
				expectStart = true
			}
		}
		if l.inline != "" {
			// An inline comment belongs to the entry on its line.
			i := len(toks) - 1
			for i > 0 && !starts[i] {
				i--
			}
			if i >= 0 {
				docs[i] = append(docs[i], Comment{Line: l.n, Text: l.inline})
			}
		}
	}

	var entries []*Entry
	for i := 0; i < len(toks); {
		j := i
		for j < len(toks) && !toks[j].period {
			j++
		}
		if j == len(toks) {
			return nil, &Error{file, toks[i].line, "entry does not end with a period"}
		}
		e, err := parseEntry(file, toks[i:j])
		if err != nil {
			return nil, err
		}
		e.Doc = docs[i]
		entries = append(entries, e)
		i = j + 1
	}
	return nest(file, entries)
}

func splitLines(file, src string, format Format) ([]line, error) {
	var out []line
	for i, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		n := i + 1
		raw = strings.ReplaceAll(raw, "\t", "    ")
		var text string
		if format == Fixed {
			if len(raw) <= 6 {
				out = append(out, line{n: n, blank: true})
				continue
			}
			ind := raw[6]
			end := len(raw)
			if end > 72 {
				if ind == '*' || ind == '/' {
					if strings.TrimSpace(raw[72:]) != "" {
						return nil, &Error{file, n, "comment runs past column 72, where cobc stops reading; continue it on the next *> line"}
					}
				}
				end = 72
			}
			switch ind {
			case '*', '/':
				t := raw[7:end]
				t = strings.TrimPrefix(t, ">")
				out = append(out, line{n: n, comment: &Comment{n, strings.TrimSpace(t)}})
				continue
			case 'D', 'd':
				out = append(out, line{n: n, blank: true})
				continue
			case '-':
				return nil, &Error{file, n, "continuation lines are not supported in a docuconf copybook"}
			case ' ':
			default:
				return nil, &Error{file, n, fmt.Sprintf("column 7 holds %q; is this a free-format copybook? (use -free)", ind)}
			}
			text = raw[7:end]
		} else {
			text = raw
			t := strings.TrimSpace(text)
			if strings.HasPrefix(t, "*>") {
				out = append(out, line{n: n, comment: &Comment{n, strings.TrimSpace(t[2:])}})
				continue
			}
			if strings.HasPrefix(t, ">>") {
				out = append(out, line{n: n, blank: true})
				continue
			}
		}
		code, inline := splitInline(text)
		if strings.TrimSpace(code) == "" {
			if inline != "" {
				out = append(out, line{n: n, comment: &Comment{n, inline}})
			} else {
				out = append(out, line{n: n, blank: true})
			}
			continue
		}
		out = append(out, line{n: n, code: code, inline: inline})
	}
	return out, nil
}

// splitInline separates a *> comment from code, outside literals.
func splitInline(s string) (code, comment string) {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '*' && i+1 < len(s) && s[i+1] == '>':
			return s[:i], strings.TrimSpace(s[i+2:])
		}
	}
	return s, ""
}

func tokenize(file string, l line) ([]token, error) {
	s := l.code
	var out []token
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == ',' || c == ';':
			i++
		case c == '"' || c == '\'' || ((c == 'X' || c == 'x' || c == 'N' || c == 'n' || c == 'Z' || c == 'z') && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\'')):
			prefix := ""
			if c != '"' && c != '\'' {
				prefix = strings.ToUpper(string(c))
				i++
			}
			q := s[i]
			var b strings.Builder
			j := i + 1
			for {
				if j >= len(s) {
					return nil, &Error{file, l.n, "literal is not closed on its line"}
				}
				if s[j] == q {
					if j+1 < len(s) && s[j+1] == q {
						b.WriteByte(q)
						j += 2
						continue
					}
					break
				}
				b.WriteByte(s[j])
				j++
			}
			text := b.String()
			if prefix == "X" {
				return nil, &Error{file, l.n, "hexadecimal literals are not supported in a docuconf copybook"}
			}
			out = append(out, token{text: text, line: l.n, literal: true})
			i = j + 1
			if i < len(s) && s[i] == '.' && (i+1 == len(s) || s[i+1] == ' ') {
				out = append(out, token{text: ".", line: l.n, period: true})
				i++
			}
		default:
			j := i
			for j < len(s) && s[j] != ' ' {
				j++
			}
			w := s[i:j]
			i = j
			period := false
			if strings.HasSuffix(w, ".") {
				w, period = w[:len(w)-1], true
			}
			w = strings.TrimRight(w, ",;")
			if w != "" {
				out = append(out, token{text: w, line: l.n})
			}
			if period {
				out = append(out, token{text: ".", line: l.n, period: true})
			}
		}
	}
	return out, nil
}

func parseEntry(file string, toks []token) (*Entry, error) {
	line := toks[0].line
	fail := func(format string, args ...any) error {
		return &Error{file, line, fmt.Sprintf(format, args...)}
	}
	lvl, err := strconv.Atoi(toks[0].text)
	if err != nil || toks[0].literal {
		return nil, fail("expected a level number, found %q", toks[0].text)
	}
	e := &Entry{Line: line, Level: lvl, Usage: "DISPLAY"}
	if lvl < 1 || (lvl > 49 && lvl != 66 && lvl != 77 && lvl != 88) {
		return nil, fail("level number %d is not 01-49, 66, 77 or 88", lvl)
	}
	if lvl == 66 {
		return nil, fail("level 66 (RENAMES) is not supported in a docuconf copybook")
	}
	i := 1
	if i < len(toks) && !toks[i].literal && !isKeyword(toks[i].text) {
		e.Name = strings.ToUpper(toks[i].text)
		i++
	}
	word := func() string {
		if i < len(toks) && !toks[i].literal {
			return strings.ToUpper(toks[i].text)
		}
		return ""
	}
	skip := func(words ...string) {
		for i < len(toks) && !toks[i].literal {
			found := false
			for _, w := range words {
				if strings.EqualFold(toks[i].text, w) {
					found = true
				}
			}
			if !found {
				return
			}
			i++
		}
	}
	for i < len(toks) {
		w := word()
		i++
		switch w {
		case "PIC", "PICTURE":
			skip("IS")
			if i >= len(toks) {
				return nil, fail("PIC needs a picture string")
			}
			e.Pic = strings.ToUpper(toks[i].text)
			i++
		case "USAGE":
			skip("IS")
			e.Usage = word()
			i++
		case "DISPLAY", "COMP", "COMP-1", "COMP-2", "COMP-3", "COMP-4", "COMP-5", "COMPUTATIONAL",
			"COMPUTATIONAL-1", "COMPUTATIONAL-2", "COMPUTATIONAL-3", "COMPUTATIONAL-4", "COMPUTATIONAL-5",
			"BINARY", "PACKED-DECIMAL", "INDEX", "POINTER", "FLOAT-SHORT", "FLOAT-LONG", "NATIONAL":
			e.Usage = strings.Replace(w, "COMPUTATIONAL", "COMP", 1)
		case "OCCURS":
			n, err := intTok(toks, i)
			if err != nil {
				return nil, fail("OCCURS needs a number of times")
			}
			i++
			e.Occurs = n
			if word() == "TO" {
				i++
				hi, err := intTok(toks, i)
				if err != nil {
					return nil, fail("OCCURS %d TO needs a number", n)
				}
				i++
				e.OccursLo, e.Occurs = n, hi
			} else {
				e.OccursLo = -1
			}
			skip("TIMES")
			if word() == "DEPENDING" {
				i++
				skip("ON")
				e.Depends = word()
				i++
			}
			for word() == "ASCENDING" || word() == "DESCENDING" || word() == "INDEXED" || word() == "KEY" || word() == "IS" || word() == "BY" {
				i++
				for i < len(toks) && !toks[i].literal && !isKeyword(toks[i].text) {
					i++
				}
			}
		case "VALUE", "VALUES":
			skip("IS", "ARE")
			for i < len(toks) {
				lit, ok := literal(toks[i])
				if !ok {
					break
				}
				i++
				if w := word(); w == "THRU" || w == "THROUGH" {
					i++
					if i >= len(toks) {
						return nil, fail("THRU needs a literal")
					}
					hi, _ := literal(toks[i])
					lit.Thru = &hi
					i++
				}
				e.Values = append(e.Values, lit)
			}
		case "REDEFINES":
			e.Redefine = word()
			i++
		case "":
			return nil, fail("unexpected literal %q", toks[i-1].text)
		default:
			// SIGN, JUSTIFIED, SYNCHRONIZED, BLANK WHEN ZERO, GLOBAL...
		}
	}
	return e, nil
}

func intTok(toks []token, i int) (int, error) {
	if i >= len(toks) || toks[i].literal {
		return 0, fmt.Errorf("no number")
	}
	return strconv.Atoi(toks[i].text)
}

// literal reads a VALUE literal: a quoted string, a number or a
// figurative constant.
func literal(t token) (Literal, bool) {
	if t.literal {
		return Literal{Text: t.text}, true
	}
	u := strings.ToUpper(t.text)
	switch u {
	case "SPACE", "SPACES":
		return Literal{Text: " "}, true
	case "ZERO", "ZEROS", "ZEROES":
		return Literal{Text: "0", Numeric: true}, true
	}
	if _, err := strconv.ParseFloat(strings.TrimPrefix(t.text, "+"), 64); err == nil {
		return Literal{Text: t.text, Numeric: true}, true
	}
	return Literal{}, false
}

var keywords = map[string]bool{
	"PIC": true, "PICTURE": true, "USAGE": true, "OCCURS": true, "VALUE": true, "VALUES": true,
	"REDEFINES": true, "COMP": true, "COMP-1": true, "COMP-2": true, "COMP-3": true, "COMP-4": true,
	"COMP-5": true, "BINARY": true, "DISPLAY": true, "PACKED-DECIMAL": true, "SIGN": true,
	"JUSTIFIED": true, "JUST": true, "SYNC": true, "SYNCHRONIZED": true, "BLANK": true, "EXTERNAL": true,
	"GLOBAL": true, "COMPUTATIONAL": true, "INDEX": true, "POINTER": true, "TIMES": true, "DEPENDING": true,
}

func isKeyword(w string) bool { return keywords[strings.ToUpper(w)] }

// nest builds the item hierarchy from level numbers.
func nest(file string, entries []*Entry) ([]*Entry, error) {
	var roots []*Entry
	var stack []*Entry
	for _, e := range entries {
		switch e.Level {
		case 1, 77:
			roots = append(roots, e)
			stack = []*Entry{e}
			continue
		case 88:
			if len(stack) == 0 {
				return nil, &Error{file, e.Line, "a level-88 condition needs an item above it"}
			}
			p := stack[len(stack)-1]
			e.Parent = p
			p.Children = append(p.Children, e)
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].Level >= e.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			return nil, &Error{file, e.Line, fmt.Sprintf("level %02d item %s is not under a level-01 record", e.Level, e.Name)}
		}
		p := stack[len(stack)-1]
		e.Parent = p
		p.Children = append(p.Children, e)
		stack = append(stack, e)
	}
	return roots, nil
}

// Picture is a parsed PICTURE string.
type Picture struct {
	Alpha     bool // X or A
	Size      int  // bytes, for alphanumeric
	Signed    bool
	IntDigits int
	Frac      int
}

// ParsePicture reads the PICTURE strings a configuration field can use:
// X(n), A(n), and numeric S9(n)V9(m). Edited pictures are rejected.
func ParsePicture(pic string) (Picture, error) {
	var p Picture
	s := strings.ToUpper(pic)
	var seq []rune
	for i := 0; i < len(s); {
		c := rune(s[i])
		i++
		n := 1
		if i < len(s) && s[i] == '(' {
			j := strings.IndexByte(s[i:], ')')
			if j < 0 {
				return p, fmt.Errorf("PIC %s: unclosed (", pic)
			}
			k, err := strconv.Atoi(s[i+1 : i+j])
			if err != nil || k < 1 {
				return p, fmt.Errorf("PIC %s: bad repeat count", pic)
			}
			n = k
			i += j + 1
		}
		for ; n > 0; n-- {
			seq = append(seq, c)
		}
	}
	alpha, numeric := 0, 0
	afterV := false
	for i, c := range seq {
		switch {
		case c == 'X' || c == 'A':
			alpha++
		case c == 'S' && i == 0:
			p.Signed = true
		case c == '9':
			numeric++
			if afterV {
				p.Frac++
			} else {
				p.IntDigits++
			}
		case c == 'V' && !afterV:
			afterV = true
		default:
			if unicode.IsPrint(c) {
				return p, fmt.Errorf("PIC %s is an edited or unsupported picture; declare the field as X(n), 9(n), S9(n) or S9(n)V9(m), and edit it for display in your program", pic)
			}
			return p, fmt.Errorf("PIC %s is not supported", pic)
		}
	}
	switch {
	case alpha > 0 && numeric == 0 && !p.Signed && !afterV:
		p.Alpha, p.Size = true, alpha
	case alpha == 0 && numeric > 0:
		if p.IntDigits+p.Frac > 38 {
			return p, fmt.Errorf("PIC %s has more than 38 digits", pic)
		}
	default:
		return p, fmt.Errorf("PIC %s mixes alphanumeric and numeric symbols", pic)
	}
	return p, nil
}

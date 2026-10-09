// Package gen turns an annotated COBOL copybook into a docuconf contract
// and a loader program.
package gen

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/docuconf/docuconf-cobol/internal/copybook"
)

// Contract types and file types (SPEC §4.3, §4.6).
const (
	tString   = "string"
	tInt      = "int"
	tFloat    = "float"
	tBool     = "bool"
	tDuration = "duration"
	tURL      = "url"
	tEnum     = "enum"
	tList     = "list"
	tKeySet   = "keySet"
	tJSON     = "json"
)

var (
	envNameRe   = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	inputNameRe = regexp.MustCompile(`^[a-z]([-a-z0-9]{0,40}[a-z0-9])?$`)
	dnsLabelRe  = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
	programRe   = regexp.MustCompile(`^[A-Z][A-Z0-9-]{0,29}[A-Z0-9]$`)
)

// Config is a copybook's configuration record.
type Config struct {
	Copybook string // file name, as COPY names it in the loader
	Record   string // the level-01 data name
	// Wrap is true when the copybook holds only the record's fields: the
	// loader declares the level-01 Record itself and COPYs the fields
	// under it, as the program does.
	Wrap    bool
	Service string
	Package string
	Program string
	// AppVersion is @app-version: the contract's metadata.appVersion.
	AppVersion string
	// Runtime is how the loader gets the docuconf runtime: "inline"
	// (the default) or "copy" (COPY DCRTWS and DCRTPD).
	Runtime  string
	Vars     []*Var
	Files    []*FileInput
	Warnings []string

	names map[string]int // every data name in the copybook, with its line
}

// Var is one environment variable, read into one field.
type Var struct {
	Env      string
	Field    string
	Line     int
	Type     string
	Required bool
	Secret   bool
	Pic      copybook.Picture
	Double   bool // COMP-1 or COMP-2: a floating-point field
	Present  string
	Default  []string // as written; nil when there is none
	// Bounds narrower than the PIC, which the loader checks too: Min and
	// Max for an int (or an int list's items), MinRat and MaxRat for a
	// float, MinNs and MaxNs for a duration.
	Min, Max       *big.Int
	MinRat, MaxRat *big.Rat
	MinNs, MaxNs   *int64
	// Length limits in characters (Unicode code points), which the
	// loader counts: MinLen and MaxLen for a string, MaxLen for a url or
	// json value, ItemMinLen and ItemMaxLen for a string list's items.
	// MaxLen and ItemMaxLen are nil when they are the PIC size, which the
	// loader checks in bytes anyway. Schemes are a url's allowed schemes.
	MinLen, MaxLen         *int
	ItemMinLen, ItemMaxLen *int
	Schemes                []string
	// Pattern is a string's @pattern compiled for the loader, or nil.
	Pattern *rxProg
	// Deprecated is the @deprecated message, which the loader prints in
	// a warning when the variable is set; ReplacedBy is @replaced-by.
	Deprecated, ReplacedBy string
	MinItems               int      // 0 when there is no minimum
	MaxItems               int      // below Occurs when @max-items narrows it, else 0
	Values                 []string // enum values
	Conds                  []string // the enum's level-88 names
	Unit                   string   // duration field unit
	UnitNs                 int64
	Encoding               string // list or duration wire encoding
	// lists
	Items     string
	Separator string
	Occurs    int
	Count     string // the field that receives the number of items
	ODO       bool   // Count is the OCCURS DEPENDING ON item

	contract map[string]any
}

// isList reports whether v is read into an OCCURS table: a list, or a
// keySet's keys.
func (v *Var) isList() bool { return v.Type == tList || v.Type == tKeySet }

// FileInput is one file input, whose effective path goes into a field.
type FileInput struct {
	Name    string
	Field   string
	Line    int
	Type    string
	Path    string
	PathEnv string
	Size    int // field bytes

	contract map[string]any
}

// Options override what the copybook says.
type Options struct {
	Format  copybook.Format
	Service string
	Program string
	Package string
	Prefix  string
	// Record picks the level-01 record of a copybook that has several.
	// For a copybook with no level-01 record, it names the 01 the loader
	// declares around the fields (default DC-RECORD).
	Record string
	// Runtime is "inline" (default) or "copy".
	Runtime string
}

// problems collects errors with their copybook line.
type problems struct {
	file string
	list []problem
}

func (p *problems) add(line int, format string, args ...any) {
	p.list = append(p.list, problem{line, fmt.Sprintf("%s:%d: %s", p.file, line, fmt.Sprintf(format, args...))})
}

type problem struct {
	line int
	text string
}

// sorted returns the problems in copybook line order.
func (p *problems) sorted() []string {
	slices.SortStableFunc(p.list, func(a, b problem) int { return a.line - b.line })
	out := make([]string, len(p.list))
	for i, x := range p.list {
		out[i] = x.text
	}
	return out
}

// Error lists every problem found in a copybook.
type Error struct{ Problems []string }

func (e *Error) Error() string { return strings.Join(e.Problems, "\n") }

// Load reads a copybook file and builds its configuration model.
func Load(path string, opts Options) (*Config, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Build(path, string(src), opts)
}

// Build builds the configuration model of a copybook's source.
func Build(path, src string, opts Options) (*Config, error) {
	name := filepath.Base(path)
	roots, warns, err := copybook.Parse(name, src, opts.Format)
	if err != nil {
		return nil, err
	}
	p := &problems{file: name}
	var recs []*copybook.Entry
	for _, r := range roots {
		if r.Level == 1 {
			recs = append(recs, r)
		}
	}
	rec, err := pickRecord(name, recs, opts.Record)
	if err != nil {
		return nil, err
	}
	c := &Config{Copybook: name, Record: rec.Name, Wrap: rec.Synthetic, Runtime: firstOf(opts.Runtime, "inline")}
	for _, w := range warns {
		c.Warnings = append(c.Warnings, w.String())
	}
	if rec.Synthetic {
		c.Record = strings.ToUpper(firstOf(opts.Record, "DC-RECORD"))
		// The record's tags go above its first field: take them from there.
		if len(rec.Children) > 0 {
			rec.Doc = liftRecordTags(rec.Children[0])
		}
	}
	if !slices.Contains([]string{"inline", "copy"}, c.Runtime) {
		return nil, &Error{[]string{fmt.Sprintf("-runtime %s: use inline or copy", c.Runtime)}}
	}
	c.names = map[string]int{}
	var collect func(e *copybook.Entry)
	collect = func(e *copybook.Entry) {
		if e.Name != "" && e.Name != "FILLER" {
			if _, dup := c.names[e.Name]; !dup {
				c.names[e.Name] = e.Line
			}
		}
		for _, ch := range e.Children {
			collect(ch)
		}
	}
	for _, r := range roots {
		collect(r)
	}
	rd, err := parseDoc(rec.Doc)
	if err != nil {
		p.add(rec.Line, "%v", err)
	}
	one := func(d doc, t string) string {
		tg, ok := d.get(t)
		if !ok {
			return ""
		}
		if len(tg.values) != 1 {
			p.add(tg.line, "%s takes one value", tg.as)
			return ""
		}
		return tg.values[0]
	}
	c.Service = firstOf(opts.Service, one(rd, "service"))
	c.Program = strings.ToUpper(firstOf(opts.Program, one(rd, "program"), defaultProgram(name, c.Record)))
	c.Package = firstOf(opts.Package, one(rd, "package"))
	c.AppVersion = one(rd, "appversion")
	prefix := strings.ToUpper(firstOf(opts.Prefix, one(rd, "prefix")))
	for _, t := range rd.tags {
		if !slices.Contains(recordTags, t.name) {
			p.add(t.line, "%s does not apply to the level-01 record; it takes @service, @program, @package, @prefix and @app-version%s", t.as, suggest(t.name, recordTags))
		}
	}
	recName := c.Record
	if rec.Synthetic {
		recName = "the first field"
	}
	switch {
	case c.Service == "":
		p.add(rec.Line, "name the service: add *> @service <name> above %s, or pass -name", recName)
	case c.Service != strings.ToLower(c.Service) && dnsLabelRe.MatchString(strings.ToLower(c.Service)):
		// Mainframe sources are upper case; a service name is a DNS label.
		c.Warnings = append(c.Warnings, fmt.Sprintf("%s:%d: warning: service name %s is lower-cased to %s, as a DNS label must be", name, rec.Line, c.Service, strings.ToLower(c.Service)))
		c.Service = strings.ToLower(c.Service)
	case !dnsLabelRe.MatchString(c.Service):
		p.add(rec.Line, "service name %q must be a DNS label such as orders-batch", c.Service)
	}
	if !programRe.MatchString(c.Program) {
		p.add(rec.Line, "program name %q must be 2-31 letters, digits and hyphens", c.Program)
	}

	// Items that other items name as their count or presence flag.
	helpers := map[string]bool{}
	var walk func(e *copybook.Entry)
	walk = func(e *copybook.Entry) {
		if e.Depends != "" {
			helpers[e.Depends] = true
		}
		if d, err := parseDoc(e.Doc); err == nil {
			for _, t := range d.tags {
				if (t.name == "count" || t.name == "present") && len(t.values) == 1 {
					helpers[strings.ToUpper(t.values[0])] = true
				}
			}
		}
		for _, ch := range e.Children {
			walk(ch)
		}
	}
	walk(rec)
	fields := map[string]*copybook.Entry{}
	var index func(e *copybook.Entry)
	index = func(e *copybook.Entry) {
		if e.Name != "" && e.Name != "FILLER" {
			fields[e.Name] = e
		}
		for _, ch := range e.Children {
			index(ch)
		}
	}
	index(rec)

	// A comment block above a count field that comes first, as the
	// DEPENDING ON item must, describes the table after it.
	var lift func(e *copybook.Entry)
	lift = func(e *copybook.Entry) {
		for i, ch := range e.Children {
			if helpers[ch.Name] && len(ch.Doc) > 0 && i+1 < len(e.Children) && len(e.Children[i+1].Doc) == 0 {
				e.Children[i+1].Doc, ch.Doc = ch.Doc, nil
			}
			lift(ch)
		}
	}
	lift(rec)
	// cobc accepts OCCURS DEPENDING ON only on the record's last item.
	var flat []*copybook.Entry
	var flatten func(e *copybook.Entry)
	flatten = func(e *copybook.Entry) {
		for _, ch := range e.Children {
			if ch.Level != 88 {
				flat = append(flat, ch)
				flatten(ch)
			}
		}
	}
	flatten(rec)
	for i, e := range flat {
		if e.Depends != "" && i != len(flat)-1 {
			p.add(e.Line, "%s: OCCURS DEPENDING ON must be on the last item of the record, as cobc requires; move it to the end, or use OCCURS %d TIMES with @count %s", e.Name, e.Occurs, e.Depends)
		}
	}
	b := &builder{p: p, c: c, prefix: prefix, fields: fields, helpers: helpers, dir: filepath.Dir(path)}
	for _, ch := range rec.Children {
		b.item(ch, "")
	}
	for h := range helpers {
		if fields[h] == nil {
			p.add(rec.Line, "%s is named as a count or presence field but is not in %s", h, rec.Name)
		}
	}
	seen := map[string]int{}
	for _, v := range c.Vars {
		if l, dup := seen[v.Env]; dup {
			p.add(v.Line, "variable %s is also read by the field at line %d", v.Env, l)
		}
		seen[v.Env] = v.Line
	}
	for _, f := range c.Files {
		if f.PathEnv != "" {
			if l, dup := seen[f.PathEnv]; dup {
				p.add(f.Line, "pathEnv %s is also a variable (line %d)", f.PathEnv, l)
			}
		}
	}
	if len(p.list) > 0 {
		return nil, &Error{p.sorted()}
	}
	return c, nil
}

// pickRecord chooses the configuration record: the only level-01 record,
// or the one -record names.
func pickRecord(file string, recs []*copybook.Entry, want string) (*copybook.Entry, error) {
	want = strings.ToUpper(want)
	var names []string
	for _, r := range recs {
		if r.Synthetic {
			return r, nil // a copybook of fields; want names the wrapper
		}
		if want != "" && r.Name == want {
			return r, nil
		}
		names = append(names, r.Name)
	}
	switch {
	case len(recs) == 0:
		return nil, &Error{[]string{fmt.Sprintf("%s: no configuration record: the copybook declares no data items", file)}}
	case want != "":
		return nil, &Error{[]string{fmt.Sprintf("%s: -record %s: the level-01 records are %s", file, want, strings.Join(names, ", "))}}
	case len(recs) > 1:
		return nil, &Error{[]string{fmt.Sprintf("%s: the copybook has %d level-01 records (%s); pick the configuration record with -record %s", file, len(recs), strings.Join(names, ", "), names[0])}}
	}
	return recs[0], nil
}

// liftRecordTags moves the record tags (@service, @prefix, @program,
// @package) out of the first field's comments, for a copybook with no
// level-01 record, and returns them as the record's comments.
func liftRecordTags(first *copybook.Entry) []copybook.Comment {
	var rec, keep []copybook.Comment
	for _, c := range first.Doc {
		t := strings.TrimSpace(c.Text)
		if strings.HasPrefix(t, "@") {
			if words, err := splitWords(t); err == nil && len(words) > 0 {
				var recWords, fieldWords []string
				var cur *[]string
				for _, w := range words {
					text := w.text
					if w.quoted {
						text = cobolLiteral(w.text)
					}
					if !w.quoted && strings.HasPrefix(w.text, "@") {
						if slices.Contains(recordTags, normTag(w.text[1:])) {
							cur = &recWords
						} else {
							cur = &fieldWords
						}
					}
					if cur != nil {
						*cur = append(*cur, text)
					}
				}
				if len(recWords) > 0 {
					rec = append(rec, copybook.Comment{Line: c.Line, Text: strings.Join(recWords, " ")})
				}
				if len(fieldWords) > 0 {
					keep = append(keep, copybook.Comment{Line: c.Line, Text: strings.Join(fieldWords, " "), Inline: c.Inline})
				}
				continue
			}
		}
		keep = append(keep, c)
	}
	first.Doc = keep
	return rec
}

// memberRe matches a name that is also a PDS member name.
var memberRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,7}$`)

// defaultProgram is the loader's PROGRAM-ID when neither @program nor
// -program gives one: for a copybook named like a PDS member (ORDCFGC.cpy),
// the member's first seven characters and L (ORDCFGCL); otherwise
// LOAD-<record>.
func defaultProgram(file, record string) string {
	base := strings.TrimSuffix(file, filepath.Ext(file))
	base = strings.ToUpper(base)
	if memberRe.MatchString(base) {
		if len(base) > 7 {
			base = base[:7]
		}
		return base + "L"
	}
	return "LOAD-" + record
}

func firstOf(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

type builder struct {
	p       *problems
	c       *Config
	prefix  string
	fields  map[string]*copybook.Entry
	helpers map[string]bool
	dir     string // the copybook's directory, for @schema files
}

func (b *builder) item(e *copybook.Entry, group string) {
	if e.Level == 88 {
		return
	}
	d, err := parseDoc(e.Doc)
	if err != nil {
		b.p.add(e.Line, "%v", err)
		return
	}
	if e.Redefine != "" {
		b.p.add(e.Line, "%s: REDEFINES is not supported in a configuration record", e.Name)
		return
	}
	if d.has("ignore") {
		return
	}
	if e.IsGroup() {
		if e.Occurs > 0 {
			b.p.add(e.Line, "%s: a table of groups is not supported; put OCCURS on an elementary item", e.Name)
			return
		}
		g := group
		if t, ok := d.get("group"); ok && len(t.values) > 0 {
			g = strings.Join(t.values, " ")
		}
		for _, t := range d.tags {
			if t.name != "group" {
				b.p.add(t.line, "%s does not apply to group item %s; it takes @group", t.as, e.Name)
			}
		}
		for _, ch := range e.Children {
			b.item(ch, g)
		}
		return
	}
	if e.Name == "" || e.Name == "FILLER" {
		return
	}
	if b.helpers[e.Name] {
		if len(d.tags) > 0 {
			b.p.add(e.Line, "%s is a count or presence field; it takes no annotations", e.Name)
		}
		return
	}
	if d.has("file") {
		b.file(e, d, group)
		return
	}
	b.variable(e, d, group)
}

// known lists the tags a variable may carry, by normalised name.
var knownVarTags = []string{"env", "desc", "details", "type", "secret", "required", "default", "min", "max",
	"minlength", "maxlength", "pattern", "schemes", "values", "minitems", "maxitems", "itemmin",
	"itemmax", "itemminlength", "itemmaxlength", "encoding", "separator", "unit", "count", "present",
	"examples", "deprecated", "replacedby", "group", "schema", "configkey", "minkeys", "maxkeys",
	"keyminlength", "keymaxlength"}

func (b *builder) variable(e *copybook.Entry, d doc, group string) {
	p := b.p
	fail := func(format string, args ...any) { p.add(e.Line, "%s: %s", e.Name, fmt.Sprintf(format, args...)) }
	for _, t := range d.tags {
		if !slices.Contains(knownVarTags, t.name) {
			p.add(t.line, "%s: unknown tag %s%s", e.Name, t.as, suggest(t.name, knownVarTags))
		}
	}
	single := func(name string) (string, bool) {
		t, ok := d.get(name)
		if !ok {
			return "", false
		}
		if len(t.values) != 1 {
			p.add(t.line, "%s: %s takes one value", e.Name, t.as)
			return "", false
		}
		return t.values[0], true
	}
	flag := func(name string) bool {
		t, ok := d.get(name)
		if ok && len(t.values) > 0 {
			p.add(t.line, "%s: %s takes no value", e.Name, t.as)
		}
		return ok
	}
	v := &Var{Field: e.Name, Line: e.Line}
	if env, ok := single("env"); ok {
		v.Env = env
		if !envNameRe.MatchString(env) {
			fail("@env %s is not an environment variable name ([A-Z][A-Z0-9_]*)", env)
		}
	} else {
		n := e.Name
		if b.prefix != "" {
			n = strings.TrimPrefix(n, b.prefix)
		}
		v.Env = strings.ReplaceAll(n, "-", "_")
		if !envNameRe.MatchString(v.Env) {
			fail("the derived variable name %s is not valid; name it with @env", v.Env)
		}
	}
	desc := d.description()
	if len([]rune(desc)) < 5 {
		fail("needs a description of at least 5 characters, as the docuconf spec requires of every input (SPEC §4.2): write a comment line above it, or @desc")
	}
	v.Required, v.Secret = flag("required"), flag("secret")
	o := map[string]any{"description": desc}
	if details, ok := d.details(); ok {
		checkDetails(details, fail)
		o["details"] = details
	}
	if v.Required {
		o["required"] = true
	}
	if v.Secret {
		o["secret"] = true
	}
	if t, ok := d.get("group"); ok {
		group = strings.Join(t.values, " ")
	}
	if group != "" {
		o["group"] = group
	}
	if dep := b.deprecated(e, d, v.Required, "variable"); dep != nil {
		o["deprecated"] = dep
		v.Deprecated = dep["message"].(string)
		v.ReplacedBy, _ = dep["replacedBy"].(string)
	}
	if s, ok := single("configkey"); ok {
		o["configKey"] = s
	}
	if t, ok := d.get("examples"); ok {
		o["examples"] = toAny(t.values)
	}

	// The field's storage decides the type, unless @type says otherwise.
	pic, err := picture(e)
	if err != nil {
		fail("%v", err)
		return
	}
	v.Pic = pic
	v.Double = e.Usage == "COMP-1" || e.Usage == "COMP-2" || e.Usage == "FLOAT-SHORT" || e.Usage == "FLOAT-LONG"
	conds := e.Conditions()
	typ, _ := single("type")
	if strings.EqualFold(typ, tKeySet) {
		typ = tKeySet
	}
	_, hasUnit := d.get("unit")
	switch {
	case typ != "":
	case hasUnit:
		typ = tDuration
	case pic.Alpha && (len(conds) > 0 || d.has("values")):
		typ = tEnum
	case pic.Alpha:
		typ = tString
	case v.Double || pic.Frac > 0:
		typ = tFloat
	default:
		typ = tInt
	}
	switch {
	case typ == tKeySet && (e.Occurs == 0 || !pic.Alpha):
		fail("a keySet is a table of keys: PIC X(n) OCCURS m, with @count")
		return
	case typ == tKeySet:
		// Always secret (SPEC section 4.3): no default, no examples.
		v.Type, v.Items, v.Secret = tKeySet, tString, true
		o["secret"] = true
		typ = tString
	case e.Occurs > 0:
		v.Items = typ
		v.Type = tList
		if typ != tString && typ != tInt {
			fail("a list (OCCURS) holds strings (PIC X) or integers (PIC 9); %s items are not supported", typ)
			return
		}
	default:
		v.Type = typ
	}
	if !slices.Contains([]string{tString, tInt, tFloat, tBool, tDuration, tURL, tEnum, tJSON}, typ) {
		fail("@type %s is not one of string, int, float, bool, duration, url, enum, json, keySet", typ)
		return
	}
	scalar := typ
	switch scalar {
	case tString, tURL, tEnum, tJSON:
		if !pic.Alpha {
			fail("a %s variable needs an alphanumeric field (PIC X(n)), not PIC %s", scalar, e.Pic)
			return
		}
	case tInt, tFloat, tDuration:
		if pic.Alpha {
			fail("a %s variable needs a numeric field (PIC 9(n), S9(n) or S9(n)V9(m)), not PIC %s", scalar, e.Pic)
			return
		}
		if scalar == tInt && (pic.Frac > 0 || v.Double) {
			fail("an int variable needs a field with no decimal places")
			return
		}
	case tBool:
		if !(pic.Alpha && pic.Size == 1) && !(!pic.Alpha && !v.Double && pic.IntDigits == 1 && pic.Frac == 0) {
			fail("a bool variable needs PIC X (set to Y or N) or PIC 9 (set to 1 or 0)")
			return
		}
	}

	allowed := map[string]bool{}
	allow := func(tags ...string) {
		for _, t := range tags {
			allowed[t] = true
		}
	}
	allow("env", "desc", "details", "type", "secret", "required", "default", "examples", "deprecated", "replacedby", "group", "present", "configkey")

	// Presence flag.
	if f, ok := single("present"); ok {
		f = strings.ToUpper(f)
		v.Present = f
		if pe := b.fields[f]; pe == nil {
			fail("@present %s is not a field of the record", f)
		} else if pp, err := picture(pe); err != nil || !pp.Alpha || pp.Size != 1 || pe.Occurs > 0 {
			fail("@present %s must be a PIC X field; the loader sets it to Y or N", f)
		}
	}

	// fieldLength is the maxLength of a value that goes into the PIC X
	// field: the field's size, or less with @max-length. The size is in
	// bytes and maxLength counts characters, so a value with multi-byte
	// characters can still be too long for the field; the loader checks.
	fieldLength := func() int64 {
		n := pic.Size
		if s, ok := single("maxlength"); ok {
			m, err := strconv.Atoi(s)
			switch {
			case err != nil || m < 0:
				fail("@max-length must be a non-negative integer")
			case m > pic.Size:
				fail("@max-length %d is more than PIC %s holds", m, e.Pic)
			default:
				n = m
				if m < pic.Size {
					v.MaxLen = &m
				}
			}
		}
		return int64(n)
	}

	switch scalar {
	case tString:
		// A list of strings takes @item-min-length and @item-max-length.
		if e.Occurs == 0 {
			allow("minlength", "maxlength", "pattern")
			o["maxLength"] = fieldLength()
			if s, ok := single("minlength"); ok {
				n, err := strconv.Atoi(s)
				if err != nil || n < 0 {
					fail("@min-length must be a non-negative integer")
				}
				o["minLength"] = int64(n)
				v.MinLen = &n
			}
			if s, ok := single("pattern"); ok {
				o["pattern"] = s
				prog, err := compileRE2(s)
				switch {
				case err != nil:
					fail("@pattern %q is not an RE2 pattern: %v", s, err)
				case prog == nil:
					b.c.Warnings = append(b.c.Warnings, fmt.Sprintf("%s:%d: warning: %s: @pattern compiles to more than the loader's %d instructions or %d character ranges; only docuconf exec checks it", b.c.Copybook, e.Line, e.Name, rxMaxInsts, rxMaxRanges))
				default:
					v.Pattern = prog
				}
			}
		}
	case tURL:
		allow("schemes", "maxlength")
		if t, ok := d.get("schemes"); ok {
			o["schemes"] = toAny(t.values)
			v.Schemes = t.values
		}
		o["maxLength"] = fieldLength()
	case tEnum:
		allow("values")
		if t, ok := d.get("values"); ok {
			if len(conds) > 0 {
				fail("give the enum values either as level-88 conditions or with @values, not both")
			}
			v.Values = t.values
		}
		for _, c := range conds {
			for _, l := range c.Values {
				if l.Numeric || l.Thru != nil {
					p.add(c.Line, "%s: level-88 %s: an enum value is a quoted string; THRU and numbers are not supported", e.Name, c.Name)
					continue
				}
				v.Values = append(v.Values, l.Text)
			}
			v.Conds = append(v.Conds, c.Name)
		}
		for _, x := range v.Values {
			if len(x) > pic.Size {
				fail("enum value %q is longer than PIC %s holds", x, e.Pic)
			}
		}
		if len(v.Values) == 0 {
			fail("an enum needs values: level-88 conditions or @values")
		}
		o["values"] = toAny(v.Values)
	case tJSON:
		// maxLength bounds the raw JSON text as received, which is what
		// goes into the field.
		allow("schema", "maxlength")
		if s, ok := single("schema"); ok {
			o["schema"] = b.schema(e, s)
		}
		o["maxLength"] = fieldLength()
	case tInt:
		allow("min", "max")
		if e.Occurs == 0 {
			picLo, picHi := intRange(pic)
			lo, hi := b.bounds(e, d, picLo, picHi, "min", "max")
			o["min"], o["max"] = json.Number(lo.String()), json.Number(hi.String())
			v.Min, v.Max = narrower(lo, picLo), narrower(hi, picHi)
		}
	case tFloat:
		allow("min", "max")
		lo, hi := floatRange(pic, v.Double)
		for _, k := range []string{"min", "max"} {
			s, ok := single(k)
			if !ok {
				continue
			}
			r, okr := new(big.Rat).SetString(s)
			if !okr {
				fail("@%s %s is not a number", k, s)
				continue
			}
			if (lo != nil && r.Cmp(lo) < 0) || (hi != nil && r.Cmp(hi) > 0) {
				fail("@%s %s is outside what PIC %s holds", k, s, e.Pic)
				continue
			}
			if exp := new(big.Rat).Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(pic.Frac)), nil))); !v.Double && !exp.IsInt() {
				fail("@%s %s has more decimal places than PIC %s holds", k, s, e.Pic)
				continue
			}
			if k == "min" {
				lo, v.MinRat = r, r
			} else {
				hi, v.MaxRat = r, r
			}
		}
		if lo != nil {
			o["min"] = ratNumber(lo, pic.Frac)
		}
		if hi != nil {
			o["max"] = ratNumber(hi, pic.Frac)
		}
	case tBool:
	case tDuration:
		allow("min", "max", "unit", "encoding")
		unit, _ := single("unit")
		ns, ok := durationUnits[unit]
		if !ok {
			fail("@unit must be one of ns, us, ms, s, m, h: the unit the field counts in")
			return
		}
		v.Unit, v.UnitNs = unit, ns
		v.Encoding = "go"
		if s, ok := single("encoding"); ok {
			v.Encoding = s
		}
		if !slices.Contains([]string{"go", "iso8601", "seconds", "timespan"}, v.Encoding) {
			fail("@encoding %s is not a duration encoding (go, iso8601, seconds, timespan)", v.Encoding)
		}
		o["encoding"] = v.Encoding
		picLo, picHi := durationRange(pic, ns)
		lo, hi := picLo, picHi
		for _, k := range []string{"min", "max"} {
			s, ok := single(k)
			if !ok {
				continue
			}
			dur, err := time.ParseDuration(s)
			if err != nil {
				fail("@%s %s is not a duration such as 30s", k, s)
				continue
			}
			if int64(dur) < picLo || int64(dur) > picHi {
				fail("@%s %s is outside what PIC %s of %s holds (%s to %s)", k, s, e.Pic, unit, formatDuration(picLo), formatDuration(picHi))
				continue
			}
			x := int64(dur)
			if k == "min" {
				lo, v.MinNs = x, &x
			} else {
				hi, v.MaxNs = x, &x
			}
		}
		o["min"], o["max"] = formatDuration(lo), formatDuration(hi)
	}

	if e.Occurs > 0 {
		b.list(e, d, v, o, single, allow)
	} else if e.Depends != "" {
		fail("DEPENDING ON needs OCCURS")
	}
	for _, t := range d.tags {
		if !allowed[t.name] && slices.Contains(knownVarTags, t.name) {
			p.add(t.line, "%s: %s does not apply to a %s variable", e.Name, t.as, v.Type)
		}
	}

	// The default is @default or, failing that, the VALUE clause.
	source, defLine := "@default", 0
	if t, ok := d.get("default"); ok {
		v.Default, defLine = t.values, t.line
	}
	if words, ok, err := valueDefault(e, v); err != nil {
		fail("%v", err)
	} else if ok && v.Default == nil {
		v.Default, source, defLine = words, "VALUE", e.Line
	} else if ok {
		fromValue, err1 := b.defaultValue(&Var{Type: v.Type, Items: v.Items, Pic: v.Pic, Double: v.Double, Unit: v.Unit, UnitNs: v.UnitNs, Occurs: v.Occurs, Default: words}, e)
		fromTag, err2 := b.defaultValue(v, e)
		if err1 == nil && err2 == nil && fmt.Sprint(fromValue) != fmt.Sprint(fromTag) {
			fail("VALUE %s and @default %s disagree; keep one of them", strings.Join(words, " "), strings.Join(v.Default, " "))
		}
	}
	if t, ok := d.get("examples"); ok && v.Secret {
		p.add(t.line, "%s: a secret has no examples, so none can leak into docs", e.Name)
	}
	if v.Default != nil {
		switch {
		case v.Secret:
			p.add(defLine, "%s: a @secret variable cannot have a default (%s); supply it from a Kubernetes Secret", e.Name, source)
		case v.Required:
			p.add(defLine, "%s: a @required variable cannot have a default (%s); drop @required, or the default", e.Name, source)
		}
		def, err := b.defaultValue(v, e)
		if err != nil {
			p.add(defLine, "%s: %s: %v", e.Name, source, err)
		} else {
			o["default"] = def
		}
	}
	o["type"] = v.Type
	v.contract = o
	b.c.Vars = append(b.c.Vars, v)
}

func (b *builder) list(e *copybook.Entry, d doc, v *Var, o map[string]any, single func(string) (string, bool), allow func(...string)) {
	fail := func(format string, args ...any) { b.p.add(e.Line, "%s: %s", e.Name, fmt.Sprintf(format, args...)) }
	// A keySet's keys are a list of strings with their own tags
	// (SPEC section 4.3): minKeys (default 1), maxKeys, keyMinLength and
	// keyMaxLength.
	ks := v.Type == tKeySet
	minTag, maxTag, minField, maxField := "minitems", "maxitems", "minItems", "maxItems"
	lenMinTag, lenMaxTag, lenMinField, lenMaxField := "itemminlength", "itemmaxlength", "itemMinLength", "itemMaxLength"
	allow("encoding", "separator", "count")
	if ks {
		minTag, maxTag, minField, maxField = "minkeys", "maxkeys", "minKeys", "maxKeys"
		lenMinTag, lenMaxTag, lenMinField, lenMaxField = "keyminlength", "keymaxlength", "keyMinLength", "keyMaxLength"
		allow("minkeys", "maxkeys")
	} else {
		allow("minitems", "maxitems", "itemmin", "itemmax")
	}
	v.Occurs = e.Occurs
	if v.Occurs > 1000 {
		fail("a loader reads at most 1000 items; OCCURS %d is too many", v.Occurs)
	}
	if !ks {
		o["items"] = v.Items
	}
	v.Encoding = "csv"
	if s, ok := single("encoding"); ok {
		v.Encoding = s
	}
	if !slices.Contains([]string{"csv", "json", "indexed"}, v.Encoding) {
		fail("@encoding %s is not a list encoding (csv, json, indexed)", v.Encoding)
	}
	o["encoding"] = v.Encoding
	if v.Encoding == "csv" {
		v.Separator = ","
		if s, ok := single("separator"); ok {
			v.Separator = s
		}
		if v.Separator == "" || len(v.Separator) > 16 {
			fail("@separator must be 1 to 16 characters")
		}
		o["separator"] = v.Separator
	} else if _, ok := d.get("separator"); ok {
		fail("@separator applies only to the csv encoding")
	}
	maxItems, minItems := v.Occurs, 0
	if e.OccursLo >= 0 {
		minItems = e.OccursLo
	}
	if ks && minItems < 1 {
		minItems = 1
	}
	if s, ok := single(maxTag); ok {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > v.Occurs {
			fail("@%s must be a number up to OCCURS %d", tagName(maxTag), v.Occurs)
		} else {
			maxItems = n
		}
	}
	if s, ok := single(minTag); ok {
		n, err := strconv.Atoi(s)
		switch {
		case err != nil || n < 0 || n > maxItems:
			fail("@%s must be a number up to the most %s, %d", tagName(minTag), map[bool]string{true: "keys", false: "items"}[ks], maxItems)
		case ks && n < 1:
			fail("@min-keys must be at least 1: a keySet always holds a key")
		default:
			minItems = n
		}
	}
	if minItems > 0 {
		o[minField] = int64(minItems)
	}
	o[maxField] = int64(maxItems)
	v.MinItems = minItems
	if maxItems < v.Occurs {
		v.MaxItems = maxItems
	}
	if v.Items == tInt {
		picLo, picHi := intRange(v.Pic)
		lo, hi := b.bounds(e, d, picLo, picHi, "itemmin", "itemmax")
		o["itemMin"], o["itemMax"] = json.Number(lo.String()), json.Number(hi.String())
		v.Min, v.Max = narrower(lo, picLo), narrower(hi, picHi)
	}
	if v.Items == tString {
		// Each item goes into one PIC X(n) entry: itemMaxLength defaults to
		// n, which counts bytes while itemMaxLength counts characters.
		allow(lenMinTag, lenMaxTag)
		hi := v.Pic.Size
		if s, ok := single(lenMaxTag); ok {
			n, err := strconv.Atoi(s)
			switch {
			case err != nil || n < 0 || (ks && n < 1):
				fail("@%s must be a positive integer", tagName(lenMaxTag))
			case n > v.Pic.Size:
				fail("@%s %d is more than PIC %s holds", tagName(lenMaxTag), n, e.Pic)
			default:
				hi = n
				if n < v.Pic.Size {
					v.ItemMaxLen = &n
				}
			}
		}
		o[lenMaxField] = int64(hi)
		if s, ok := single(lenMinTag); ok {
			n, err := strconv.Atoi(s)
			switch {
			case err != nil || n < 0:
				fail("@%s must be a non-negative integer", tagName(lenMinTag))
			case n > hi:
				fail("@%s %d is above the %s maximum length %d", tagName(lenMinTag), n, map[bool]string{true: "key", false: "item"}[ks], hi)
			default:
				o[lenMinField] = int64(n)
				v.ItemMinLen = &n
			}
		}
		if ks && (v.ItemMinLen == nil || *v.ItemMinLen < 1) {
			// An empty key is out of range whatever the bounds: a stray
			// separator must not become a key anyone can match.
			one := 1
			v.ItemMinLen = &one
		}
	}
	// The number of items goes in the DEPENDING ON item, or in @count.
	count, hasCount := single("count")
	switch {
	case e.Depends != "" && hasCount:
		fail("give the item count with DEPENDING ON or @count, not both")
	case e.Depends != "":
		v.Count, v.ODO = e.Depends, true
	case hasCount:
		v.Count = strings.ToUpper(count)
	default:
		fail("a list needs a field for its number of items: OCCURS 1 TO %d DEPENDING ON <field>, or @count <field>", v.Occurs)
		return
	}
	ce := b.fields[v.Count]
	if ce == nil {
		fail("count field %s is not in the record", v.Count)
		return
	}
	cp, err := picture(ce)
	if err != nil || cp.Alpha || cp.Frac > 0 || ce.Occurs > 0 || len(strconv.Itoa(v.Occurs)) > cp.IntDigits {
		fail("count field %s must be an integer field that holds %d", v.Count, v.Occurs)
	}
}

// narrower returns x when it differs from the PIC's limit, else nil.
func narrower(x, pic *big.Int) *big.Int {
	if x.Cmp(pic) == 0 {
		return nil
	}
	return x
}

// bounds narrows the PIC range with @min and @max (or the item tags).
func (b *builder) bounds(e *copybook.Entry, d doc, picLo, picHi *big.Int, minTag, maxTag string) (*big.Int, *big.Int) {
	lo, hi := picLo, picHi
	for _, k := range []string{minTag, maxTag} {
		t, ok := d.get(k)
		if !ok {
			continue
		}
		if len(t.values) != 1 {
			b.p.add(t.line, "%s: %s takes one value", e.Name, t.as)
			continue
		}
		n, okn := new(big.Int).SetString(t.values[0], 10)
		if !okn {
			b.p.add(t.line, "%s: %s %s is not an integer", e.Name, t.as, t.values[0])
			continue
		}
		if n.Cmp(picLo) < 0 || n.Cmp(picHi) > 0 {
			hint := ""
			if n.Sign() < 0 && picLo.Sign() == 0 {
				hint = "; PIC 9 is unsigned, use S9 for negative values"
			}
			b.p.add(t.line, "%s: %s %s is outside what PIC %s holds (%s to %s)%s", e.Name, t.as, n, e.Pic, picLo, picHi, hint)
			continue
		}
		if k == minTag {
			lo = n
		} else {
			hi = n
		}
	}
	if lo.Cmp(hi) > 0 {
		b.p.add(e.Line, "%s: the minimum %s is above the maximum %s", e.Name, lo, hi)
	}
	return lo, hi
}

// schema reads a JSON Schema file, relative to the copybook.
func (b *builder) schema(e *copybook.Entry, file string) any {
	if !filepath.IsAbs(file) {
		file = filepath.Join(b.dir, file)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		b.p.add(e.Line, "%s: @schema: %v", e.Name, err)
		return nil
	}
	var s any
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&s); err != nil {
		b.p.add(e.Line, "%s: @schema %s is not JSON: %v", e.Name, file, err)
	}
	return s
}

func (b *builder) defaultValue(v *Var, e *copybook.Entry) (any, error) {
	one := func() (string, error) {
		if len(v.Default) != 1 {
			return "", fmt.Errorf("takes one value")
		}
		return v.Default[0], nil
	}
	switch v.Type {
	case tString, tURL, tEnum:
		s, err := one()
		if err != nil {
			return nil, err
		}
		if len(s) > v.Pic.Size {
			return nil, fmt.Errorf("%q is longer than PIC %s holds", s, e.Pic)
		}
		return s, nil
	case tJSON:
		s, err := one()
		if err != nil {
			return nil, err
		}
		if len(s) > v.Pic.Size {
			return nil, fmt.Errorf("is longer than PIC %s holds", e.Pic)
		}
		var x any
		dec := json.NewDecoder(strings.NewReader(s))
		dec.UseNumber()
		if err := dec.Decode(&x); err != nil {
			return nil, fmt.Errorf("%q is not JSON", s)
		}
		return x, nil
	case tInt:
		s, err := one()
		if err != nil {
			return nil, err
		}
		n, ok := new(big.Int).SetString(strings.TrimPrefix(s, "+"), 10)
		if !ok {
			return nil, fmt.Errorf("%q is not an integer", s)
		}
		return json.Number(n.String()), nil
	case tFloat:
		s, err := one()
		if err != nil {
			return nil, err
		}
		r, ok := new(big.Rat).SetString(strings.TrimPrefix(s, "+"))
		if !ok {
			return nil, fmt.Errorf("%q is not a number", s)
		}
		if !v.Double && !new(big.Rat).Mul(r, new(big.Rat).SetInt(pow10(v.Pic.Frac))).IsInt() {
			return nil, fmt.Errorf("%s has more decimal places than PIC %s holds", s, e.Pic)
		}
		return json.Number(ratNumber(r, max(v.Pic.Frac, decimals(s)))), nil
	case tBool:
		s, err := one()
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(s) {
		case "true", "y", "1":
			return true, nil
		case "false", "n", "0":
			return false, nil
		}
		return nil, fmt.Errorf("%q is not true or false (or Y, N, 1, 0)", s)
	case tDuration:
		s, err := one()
		if err != nil {
			return nil, err
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("%q is not a duration such as 30s", s)
		}
		// The field holds multiples of unit / 10^Frac.
		if new(big.Int).Rem(new(big.Int).Mul(big.NewInt(int64(d)), pow10(v.Pic.Frac)), big.NewInt(v.UnitNs)).Sign() != 0 {
			if v.Pic.Frac == 0 {
				return nil, fmt.Errorf("%s is not a whole number of %s, which PIC %s counts in", s, v.Unit, e.Pic)
			}
			return nil, fmt.Errorf("%s is finer than PIC %s of %s holds", s, e.Pic, v.Unit)
		}
		return formatDuration(int64(d)), nil
	case tList, tKeySet:
		if len(v.Default) > v.Occurs {
			return nil, fmt.Errorf("has %d items; OCCURS %d holds fewer", len(v.Default), v.Occurs)
		}
		out := make([]any, len(v.Default))
		for i, s := range v.Default {
			if v.Items == tInt {
				if _, ok := new(big.Int).SetString(s, 10); !ok {
					return nil, fmt.Errorf("item %q is not an integer", s)
				}
				out[i] = json.Number(s)
			} else {
				if len(s) > v.Pic.Size {
					return nil, fmt.Errorf("item %q is longer than PIC %s holds", s, e.Pic)
				}
				out[i] = s
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported")
}

func pow10(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

// decimals counts the digits after the point in a number as written.
func decimals(s string) int {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return len(s) - i - 1
	}
	return 0
}

// valueDefault reads the default a VALUE clause gives a field, in the
// words @default would take. SPACES, ZEROS, LOW-VALUES and HIGH-VALUES
// only initialise the field, so they give no default; nor does a field
// without VALUE.
func valueDefault(e *copybook.Entry, v *Var) ([]string, bool, error) {
	if len(e.Values) == 0 || e.Values[0].Figurative {
		return nil, false, nil
	}
	lit := e.Values[0]
	if len(e.Values) > 1 || lit.Thru != nil {
		return nil, false, fmt.Errorf("VALUE takes one literal on a configuration field")
	}
	if v.isList() {
		return nil, false, fmt.Errorf("VALUE %s on an OCCURS table sets every item; give the list's default with @default <item>...", lit.Text)
	}
	text := lit.Text
	switch v.Type {
	case tInt, tFloat:
		if !lit.Numeric {
			return nil, false, fmt.Errorf("VALUE %q is not a number", text)
		}
	case tBool:
		// Y or N in a PIC X, 1 or 0 in a PIC 9: defaultValue reads them.
	case tDuration:
		if !lit.Numeric {
			return nil, false, fmt.Errorf("VALUE %q is not a number of %s", text, v.Unit)
		}
		r, ok := new(big.Rat).SetString(strings.TrimPrefix(text, "+"))
		if !ok {
			return nil, false, fmt.Errorf("VALUE %s is not a number", text)
		}
		ns := new(big.Rat).Mul(r, new(big.Rat).SetInt64(v.UnitNs))
		if !ns.IsInt() || !ns.Num().IsInt64() {
			return nil, false, fmt.Errorf("VALUE %s %s is not a whole number of nanoseconds", text, v.Unit)
		}
		text = formatDuration(ns.Num().Int64())
	}
	return []string{text}, true, nil
}

var knownFileTags = []string{"file", "type", "desc", "details", "required", "secret", "path", "pathenv", "reload",
	"maxsize", "group", "deprecated", "replacedby", "format", "schema", "dnsnames", "keyalgorithms", "minremaining",
	"requireca", "mincertificates", "passwordvar", "pattern", "minlength", "maxlength"}

var fileTypeTags = map[string][]string{
	"text":     {"pattern", "minlength", "maxlength"},
	"binary":   {},
	"config":   {"format", "schema"},
	"caBundle": {"mincertificates"},
	"tls":      {"dnsnames", "keyalgorithms", "minremaining", "requireca"},
	"keystore": {"format", "passwordvar"},
}

func (b *builder) file(e *copybook.Entry, d doc, group string) {
	p := b.p
	fail := func(format string, args ...any) { p.add(e.Line, "%s: %s", e.Name, fmt.Sprintf(format, args...)) }
	ft, _ := d.get("file")
	f := &FileInput{Field: e.Name, Line: e.Line, Type: "text"}
	switch len(ft.values) {
	case 1:
		f.Name = ft.values[0]
	case 2:
		f.Name, f.Type = ft.values[0], ft.values[1]
	default:
		fail("@file takes an input name and an optional type: @file orders text")
		return
	}
	if t, ok := d.get("type"); ok && len(t.values) == 1 {
		f.Type = t.values[0]
	}
	if !inputNameRe.MatchString(f.Name) {
		fail("file input name %q must be a DNS label such as orders-input", f.Name)
	}
	specific, ok := fileTypeTags[f.Type]
	if !ok {
		fail("file type %s is not one of text, binary, config, caBundle, tls, keystore", f.Type)
		return
	}
	pic, err := picture(e)
	if err != nil || !pic.Alpha || e.Occurs > 0 {
		fail("a file input's field receives its path, so it must be PIC X(n)")
		return
	}
	f.Size = pic.Size
	single := func(name string) (string, bool) {
		t, ok := d.get(name)
		if !ok {
			return "", false
		}
		if len(t.values) != 1 {
			p.add(t.line, "%s: %s takes one value", e.Name, t.as)
			return "", false
		}
		return t.values[0], true
	}
	for _, t := range d.tags {
		switch {
		case !slices.Contains(knownFileTags, t.name):
			p.add(t.line, "%s: unknown tag %s%s", e.Name, t.as, suggest(t.name, knownFileTags))
		case slices.Contains([]string{"format", "schema", "dnsnames", "keyalgorithms", "minremaining", "requireca",
			"mincertificates", "passwordvar", "pattern", "minlength", "maxlength"}, t.name) && !slices.Contains(specific, t.name):
			p.add(t.line, "%s: %s does not apply to a %s file", e.Name, t.as, f.Type)
		}
	}
	desc := d.description()
	if len([]rune(desc)) < 5 {
		fail("needs a description of at least 5 characters, as the docuconf spec requires of every input (SPEC §4.2): write a comment line above it, or @desc")
	}
	o := map[string]any{"type": f.Type, "description": desc}
	if details, ok := d.details(); ok {
		checkDetails(details, fail)
		o["details"] = details
	}
	if d.has("required") {
		o["required"] = true
	}
	if d.has("secret") || f.Type == "tls" || f.Type == "keystore" {
		o["secret"] = true
	}
	f.Path, _ = single("path")
	if f.Path == "" {
		fail("a file input needs @path, the absolute path the platform mounts it at")
	}
	if len(f.Path) > f.Size {
		fail("@path %s is longer than PIC %s holds", f.Path, e.Pic)
	}
	o["path"] = f.Path
	if s, ok := single("pathenv"); ok {
		f.PathEnv = s
		o["pathEnv"] = s
	}
	if s, ok := single("reload"); ok {
		if s == "watch" {
			fail("@reload watch needs the program to reload the file itself; a COBOL loader reads paths once, so use restart")
		}
		o["reload"] = s
	}
	if s, ok := single("maxsize"); ok {
		n, err := parseSize(s)
		if err != nil {
			fail("@max-size: %v", err)
		}
		o["maxSize"] = n
	}
	if t, ok := d.get("group"); ok {
		group = strings.Join(t.values, " ")
	}
	if group != "" {
		o["group"] = group
	}
	if dep := b.deprecated(e, d, d.has("required"), "file input"); dep != nil {
		o["deprecated"] = dep
	}
	switch f.Type {
	case "config":
		format, ok := single("format")
		if !ok {
			switch filepath.Ext(f.Path) {
			case ".json":
				format = "json"
			case ".yaml", ".yml":
				format = "yaml"
			}
		}
		o["format"] = format
		if s, ok := single("schema"); ok {
			o["schema"] = b.schema(e, s)
		}
	case "keystore":
		o["format"] = "pkcs12"
		if s, ok := single("format"); ok {
			o["format"] = s
		}
		if s, ok := single("passwordvar"); ok {
			o["passwordVar"] = s
		}
	case "tls":
		if t, ok := d.get("dnsnames"); ok {
			o["dnsNames"] = toAny(t.values)
		}
		if t, ok := d.get("keyalgorithms"); ok {
			o["keyAlgorithms"] = toAny(t.values)
		}
		if s, ok := single("minremaining"); ok {
			o["minRemaining"] = s
		}
		if d.has("requireca") {
			o["requireCA"] = true
		}
	case "caBundle":
		if s, ok := single("mincertificates"); ok {
			o["minCertificates"] = json.Number(s)
		}
	case "text":
		if s, ok := single("pattern"); ok {
			o["pattern"] = s
		}
		for _, k := range []string{"minLength", "maxLength"} {
			if s, ok := single(strings.ToLower(k)); ok {
				o[k] = json.Number(s)
			}
		}
	}
	f.contract = o
	b.c.Files = append(b.c.Files, f)
}

func picture(e *copybook.Entry) (copybook.Picture, error) {
	switch e.Usage {
	case "COMP-1", "COMP-2", "FLOAT-SHORT", "FLOAT-LONG":
		if e.Pic != "" {
			return copybook.Picture{}, fmt.Errorf("a %s field has no PIC", e.Usage)
		}
		return copybook.Picture{Signed: true}, nil
	case "INDEX", "POINTER", "NATIONAL":
		return copybook.Picture{}, fmt.Errorf("USAGE %s is not supported for a configuration field", e.Usage)
	}
	if e.Pic == "" {
		return copybook.Picture{}, fmt.Errorf("an elementary item needs a PIC clause")
	}
	return copybook.ParsePicture(e.Pic)
}

var maxInt64 = big.NewInt(math.MaxInt64)
var minInt64 = big.NewInt(math.MinInt64)

// intRange is what an integer field holds, within 64 bits.
func intRange(p copybook.Picture) (*big.Int, *big.Int) {
	hi := new(big.Int).Sub(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(p.IntDigits)), nil), big.NewInt(1))
	if hi.Cmp(maxInt64) > 0 {
		hi = new(big.Int).Set(maxInt64)
	}
	lo := new(big.Int)
	if p.Signed {
		lo = new(big.Int).Neg(hi)
		if p.IntDigits >= 19 {
			lo = new(big.Int).Set(minInt64)
		}
	}
	return lo, hi
}

// floatRange is what a decimal field holds; nil for a floating-point one.
func floatRange(p copybook.Picture, double bool) (*big.Rat, *big.Rat) {
	if double {
		return nil, nil
	}
	ten := func(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }
	num := new(big.Int).Sub(ten(p.IntDigits+p.Frac), big.NewInt(1))
	hi := new(big.Rat).SetFrac(num, ten(p.Frac))
	lo := new(big.Rat)
	if p.Signed {
		lo = new(big.Rat).Neg(hi)
	}
	return lo, hi
}

func ratNumber(r *big.Rat, frac int) json.Number {
	s := r.FloatString(frac)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return json.Number(s)
}

var durationUnits = map[string]int64{
	"ns": 1, "us": 1e3, "ms": 1e6, "s": 1e9, "m": 60e9, "h": 3600e9,
}

// durationRange is what a field counting in units of ns nanoseconds
// holds, within the 64-bit range of a Go duration. An unsigned field
// starts at 0.
func durationRange(p copybook.Picture, ns int64) (int64, int64) {
	ten := func(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }
	steps := new(big.Int).Sub(ten(p.IntDigits+p.Frac), big.NewInt(1))
	hiB := new(big.Int).Mul(steps, big.NewInt(ns))
	hiB.Quo(hiB, ten(p.Frac))
	hi := int64(math.MaxInt64)
	if hiB.IsInt64() {
		hi = hiB.Int64()
	}
	lo := int64(0)
	if p.Signed {
		lo = -hi
	}
	return lo, hi
}

// formatDuration writes d in the contract's form: units from h to ns,
// with no fractions (1m39s999ms), which #Duration accepts.
func formatDuration(d int64) string {
	if d == 0 {
		return "0s"
	}
	var b strings.Builder
	u := uint64(d)
	if d < 0 {
		b.WriteByte('-')
		u = uint64(-d)
	}
	for _, x := range []struct {
		n    uint64
		name string
	}{{3600e9, "h"}, {60e9, "m"}, {1e9, "s"}, {1e6, "ms"}, {1e3, "us"}, {1, "ns"}} {
		if q := u / x.n; q > 0 {
			fmt.Fprintf(&b, "%d%s", q, x.name)
			u -= q * x.n
		}
	}
	return b.String()
}

func parseSize(s string) (int64, error) {
	mult := int64(1)
	for suffix, m := range map[string]int64{"Ki": 1 << 10, "Mi": 1 << 20, "Gi": 1 << 30} {
		if strings.HasSuffix(s, suffix) {
			s, mult = strings.TrimSuffix(s, suffix), m
			break
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a size in bytes (Ki, Mi and Gi suffixes are allowed)", s)
	}
	return n * mult, nil
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// tagName is a normalised tag as the README writes it: @key-min-length.
func tagName(norm string) string {
	return map[string]string{
		"minitems": "min-items", "maxitems": "max-items", "minkeys": "min-keys", "maxkeys": "max-keys",
		"itemminlength": "item-min-length", "itemmaxlength": "item-max-length",
		"keyminlength": "key-min-length", "keymaxlength": "key-max-length",
	}[norm]
}

// maxDeprecation is the most characters a deprecation message may have
// (SPEC section 4.2).
const maxDeprecation = 500

// deprecated reads @deprecated "<message>" and @replaced-by <NAME> into
// the contract's deprecated object, or nil when the input is not
// deprecated. The message says what to use instead, or why the input is
// going away: not blank, and at most 500 characters. A required input
// cannot be deprecated, since the platform could not stop setting it.
func (b *builder) deprecated(e *copybook.Entry, d doc, required bool, what string) map[string]any {
	dt, ok := d.get("deprecated")
	rt, hasRB := d.get("replacedby")
	if !ok {
		if hasRB {
			b.p.add(rt.line, "%s: %s needs @deprecated", e.Name, rt.as)
		}
		return nil
	}
	if len(dt.values) != 1 {
		b.p.add(dt.line, "%s: %s takes one value, the message in quotes: @deprecated \"Use PORT instead\"", e.Name, dt.as)
		return nil
	}
	msg := dt.values[0]
	switch {
	case strings.TrimSpace(msg) == "":
		b.p.add(dt.line, "%s: %s must say what to use instead, or why the %s is going away", e.Name, dt.as, what)
	case utf8.RuneCountInString(msg) > maxDeprecation:
		b.p.add(dt.line, "%s: %s has %d characters; a deprecation message has at most %d", e.Name, dt.as, utf8.RuneCountInString(msg), maxDeprecation)
	}
	if required {
		b.p.add(dt.line, "%s: a @required %s cannot be @deprecated: deprecating it asks the platform to stop setting it", e.Name, what)
	}
	dep := map[string]any{"message": msg}
	if hasRB {
		switch {
		case len(rt.values) != 1:
			b.p.add(rt.line, "%s: %s takes one name", e.Name, rt.as)
		case what == "variable" && !envNameRe.MatchString(rt.values[0]):
			b.p.add(rt.line, "%s: %s %s is not a variable name ([A-Z][A-Z0-9_]*)", e.Name, rt.as, rt.values[0])
		default:
			dep["replacedBy"] = rt.values[0]
		}
	}
	return dep
}

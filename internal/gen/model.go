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
	Service  string
	Package  string
	Program  string
	Vars     []*Var
	Files    []*FileInput
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
	Values   []string // enum values
	Conds    []string // the enum's level-88 names
	Unit     string   // duration field unit
	UnitNs   int64
	Encoding string // list or duration wire encoding
	// lists
	Items     string
	Separator string
	Occurs    int
	Count     string // the field that receives the number of items
	ODO       bool   // Count is the OCCURS DEPENDING ON item

	contract map[string]any
}

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
}

// problems collects errors with their copybook line.
type problems struct {
	file string
	list []string
}

func (p *problems) add(line int, format string, args ...any) {
	p.list = append(p.list, fmt.Sprintf("%s:%d: %s", p.file, line, fmt.Sprintf(format, args...)))
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
	roots, err := copybook.Parse(name, src, opts.Format)
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
	if len(recs) != 1 {
		return nil, &Error{[]string{fmt.Sprintf("%s: expected one level-01 configuration record, found %d", name, len(recs))}}
	}
	rec := recs[0]
	rd, err := parseDoc(rec.Doc)
	if err != nil {
		p.add(rec.Line, "%v", err)
	}
	c := &Config{Copybook: name, Record: rec.Name}
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
	c.Program = strings.ToUpper(firstOf(opts.Program, one(rd, "program"), "LOAD-"+rec.Name))
	c.Package = firstOf(opts.Package, one(rd, "package"))
	prefix := strings.ToUpper(firstOf(opts.Prefix, one(rd, "prefix")))
	for _, t := range rd.tags {
		if !slices.Contains([]string{"service", "program", "package", "prefix"}, t.name) {
			p.add(t.line, "%s does not apply to the level-01 record; it takes @service, @program, @package and @prefix", t.as)
		}
	}
	switch {
	case c.Service == "":
		p.add(rec.Line, "name the service: add *> @service <name> above %s, or pass -name", rec.Name)
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
		return nil, &Error{p.list}
	}
	return c, nil
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
var knownVarTags = []string{"env", "desc", "type", "secret", "required", "default", "min", "max",
	"minlength", "maxlength", "pattern", "schemes", "values", "minitems", "maxitems", "itemmin",
	"itemmax", "itemminlength", "itemmaxlength", "encoding", "separator", "unit", "count", "present",
	"examples", "deprecated", "group", "schema", "configkey"}

func (b *builder) variable(e *copybook.Entry, d doc, group string) {
	p := b.p
	fail := func(format string, args ...any) { p.add(e.Line, "%s: %s", e.Name, fmt.Sprintf(format, args...)) }
	for _, t := range d.tags {
		if !slices.Contains(knownVarTags, t.name) {
			p.add(t.line, "%s: unknown tag %s", e.Name, t.as)
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
		fail("needs a description of at least 5 characters: write a comment line above it, or @desc")
	}
	v.Required, v.Secret = flag("required"), flag("secret")
	o := map[string]any{"description": desc}
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
	if s, ok := single("deprecated"); ok {
		o["deprecated"] = map[string]any{"message": s}
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
	if e.Occurs > 0 {
		v.Items = typ
		v.Type = tList
		if typ != tString && typ != tInt {
			fail("a list (OCCURS) holds strings (PIC X) or integers (PIC 9); %s items are not supported", typ)
			return
		}
	} else {
		v.Type = typ
	}
	if !slices.Contains([]string{tString, tInt, tFloat, tBool, tDuration, tURL, tEnum, tJSON}, typ) {
		fail("@type %s is not one of string, int, float, bool, duration, url, enum, json", typ)
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
	allow("env", "desc", "type", "secret", "required", "default", "examples", "deprecated", "group", "present", "configkey")

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
			}
			if s, ok := single("pattern"); ok {
				o["pattern"] = s
			}
		}
	case tURL:
		allow("schemes", "maxlength")
		if t, ok := d.get("schemes"); ok {
			o["schemes"] = toAny(t.values)
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
			lo, hi := intRange(pic)
			lo, hi = b.bounds(e, d, lo, hi, "min", "max")
			o["min"], o["max"] = json.Number(lo.String()), json.Number(hi.String())
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
			if k == "min" {
				lo = r
			} else {
				hi = r
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
		lo, hi := durationRange(pic, ns)
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
			if int64(dur) < lo || int64(dur) > hi {
				fail("@%s %s is outside what PIC %s of %s holds (%s to %s)", k, s, e.Pic, unit, formatDuration(lo), formatDuration(hi))
				continue
			}
			if k == "min" {
				lo = int64(dur)
			} else {
				hi = int64(dur)
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

	if t, ok := d.get("default"); ok {
		if v.Required || v.Secret {
			p.add(t.line, "%s: a required or secret variable has no default", e.Name)
		}
		v.Default = t.values
		def, err := b.defaultValue(v, e)
		if err != nil {
			p.add(t.line, "%s: @default: %v", e.Name, err)
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
	allow("encoding", "separator", "minitems", "maxitems", "count", "itemmin", "itemmax")
	v.Occurs = e.Occurs
	if v.Occurs > 1000 {
		fail("a loader reads at most 1000 items; OCCURS %d is too many", v.Occurs)
	}
	o["items"] = v.Items
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
	if s, ok := single("maxitems"); ok {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > v.Occurs {
			fail("@max-items must be a number up to OCCURS %d", v.Occurs)
		} else {
			maxItems = n
		}
	}
	if s, ok := single("minitems"); ok {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > maxItems {
			fail("@min-items must be a number up to the table size")
		} else {
			minItems = n
		}
	}
	if minItems > 0 {
		o["minItems"] = int64(minItems)
	}
	o["maxItems"] = int64(maxItems)
	if v.Items == tInt {
		lo, hi := intRange(v.Pic)
		lo, hi = b.bounds(e, d, lo, hi, "itemmin", "itemmax")
		o["itemMin"], o["itemMax"] = json.Number(lo.String()), json.Number(hi.String())
	}
	if v.Items == tString {
		// Each item goes into one PIC X(n) entry: itemMaxLength defaults to
		// n, which counts bytes while itemMaxLength counts characters.
		allow("itemminlength", "itemmaxlength")
		hi := v.Pic.Size
		if s, ok := single("itemmaxlength"); ok {
			n, err := strconv.Atoi(s)
			switch {
			case err != nil || n < 0:
				fail("@item-max-length must be a non-negative integer")
			case n > v.Pic.Size:
				fail("@item-max-length %d is more than PIC %s holds", n, e.Pic)
			default:
				hi = n
			}
		}
		o["itemMaxLength"] = int64(hi)
		if s, ok := single("itemminlength"); ok {
			n, err := strconv.Atoi(s)
			switch {
			case err != nil || n < 0:
				fail("@item-min-length must be a non-negative integer")
			case n > hi:
				fail("@item-min-length %d is above the item maximum length %d", n, hi)
			default:
				o["itemMinLength"] = int64(n)
			}
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

// bounds narrows the PIC range with @min and @max (or the item tags).
func (b *builder) bounds(e *copybook.Entry, d doc, lo, hi *big.Int, minTag, maxTag string) (*big.Int, *big.Int) {
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
		if n.Cmp(lo) < 0 || n.Cmp(hi) > 0 {
			hint := ""
			if n.Sign() < 0 && lo.Sign() == 0 {
				hint = "; PIC 9 is unsigned, use S9 for negative values"
			}
			b.p.add(t.line, "%s: %s %s is outside what PIC %s holds (%s to %s)%s", e.Name, t.as, n, e.Pic, lo, hi, hint)
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
	case tInt, tFloat:
		s, err := one()
		if err != nil {
			return nil, err
		}
		if _, ok := new(big.Rat).SetString(s); !ok {
			return nil, fmt.Errorf("%q is not a number", s)
		}
		return json.Number(s), nil
	case tBool:
		s, err := one()
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(s) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return nil, fmt.Errorf("%q is not true or false", s)
	case tDuration:
		s, err := one()
		if err != nil {
			return nil, err
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("%q is not a duration such as 30s", s)
		}
		if int64(d)%v.UnitNs != 0 && v.Pic.Frac == 0 {
			return nil, fmt.Errorf("%s is not a whole number of %s", s, v.Unit)
		}
		return formatDuration(int64(d)), nil
	case tList:
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

var knownFileTags = []string{"file", "type", "desc", "required", "secret", "path", "pathenv", "reload",
	"maxsize", "group", "deprecated", "format", "schema", "dnsnames", "keyalgorithms", "minremaining",
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
			p.add(t.line, "%s: unknown tag %s", e.Name, t.as)
		case slices.Contains([]string{"format", "schema", "dnsnames", "keyalgorithms", "minremaining", "requireca",
			"mincertificates", "passwordvar", "pattern", "minlength", "maxlength"}, t.name) && !slices.Contains(specific, t.name):
			p.add(t.line, "%s: %s does not apply to a %s file", e.Name, t.as, f.Type)
		}
	}
	desc := d.description()
	if len([]rune(desc)) < 5 {
		fail("needs a description of at least 5 characters: write a comment line above it, or @desc")
	}
	o := map[string]any{"type": f.Type, "description": desc}
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
	if s, ok := single("deprecated"); ok {
		o["deprecated"] = map[string]any{"message": s}
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

package copybook

import (
	"strings"
	"testing"
)

const fixed = `      *> Settings.
      *> @service demo
       01  DEMO-CONFIG.
      *> Port to listen on
      *> @min 1
           05  CFG-PORT            PIC 9(5).
           05  CFG-LEVEL           PIC X(5).    *> @default info
               88  LEVEL-INFO      VALUE "info".
               88  LEVEL-DEBUG     VALUE 'de''bug'.

      *> Not attached: a blank line follows.

           05  CFG-ITEMS           PIC X(10)
                   OCCURS 1 TO 4 TIMES DEPENDING ON CFG-N.
           05  CFG-RATIO           PIC S9(3)V99 COMP-3.
`

func TestParseFixed(t *testing.T) {
	roots, _, err := Parse("demo.cpy", fixed, Fixed)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0].Name != "DEMO-CONFIG" || len(roots[0].Children) != 4 {
		t.Fatalf("roots: %+v", roots)
	}
	rec := roots[0]
	if got := rec.Doc; len(got) != 2 || got[1].Text != "@service demo" {
		t.Errorf("record doc: %+v", got)
	}
	port := rec.Children[0]
	if port.Pic != "9(5)" || len(port.Doc) != 2 || port.Doc[0].Text != "Port to listen on" || port.Line != 6 {
		t.Errorf("port: %+v", port)
	}
	level := rec.Children[1]
	if len(level.Doc) != 1 || level.Doc[0].Text != "@default info" {
		t.Errorf("inline comment: %+v", level.Doc)
	}
	conds := level.Conditions()
	if len(conds) != 2 || conds[0].Values[0].Text != "info" || conds[1].Values[0].Text != "de'bug" {
		t.Errorf("conditions: %+v %+v", conds[0], conds[1])
	}
	items := rec.Children[2]
	if items.Occurs != 4 || items.OccursLo != 1 || items.Depends != "CFG-N" || len(items.Doc) != 0 {
		t.Errorf("items: %+v", items)
	}
	ratio := rec.Children[3]
	if ratio.Usage != "COMP-3" {
		t.Errorf("ratio usage %q", ratio.Usage)
	}
	p, err := ParsePicture(ratio.Pic)
	if err != nil || !p.Signed || p.IntDigits != 3 || p.Frac != 2 {
		t.Errorf("picture %+v %v", p, err)
	}
}

func TestParseFree(t *testing.T) {
	var free strings.Builder
	for _, l := range strings.Split(fixed, "\n") {
		free.WriteString(strings.TrimLeft(l, " ") + "\n")
	}
	got, _, err := Parse("demo.cpy", free.String(), Free)
	if err != nil {
		t.Fatal(err)
	}
	want, _, _ := Parse("demo.cpy", fixed, Fixed)
	if len(got[0].Children) != len(want[0].Children) || got[0].Children[1].Doc[0].Text != "@default info" {
		t.Fatalf("free format parsed differently: %+v", got[0])
	}
}

func TestFixedFormatErrors(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"01  DEMO-CONFIG.\n", "column 7 holds"},
		{"       01  A.\n      -    \"x\".\n", "continuation lines are not supported"},
		{"       01  A\n", "does not end with a period"},
		{"       77  A PIC X.\n       05  B PIC X.\n", "is not under a level-01 record"},
	} {
		_, _, err := Parse("demo.cpy", c.src, Fixed)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: got %v, want %q", c.src, err, c.want)
		}
	}
}

// Columns 73-80 are the identification area, ignored as cobc ignores
// them, in comment lines as in code lines.
func TestIdentificationArea(t *testing.T) {
	src := "" +
		"000100* @SERVICE ORDERS-BATCH  @PREFIX CFG-                             ORDCFGC\n" +
		"000200 01  ORDERS-CONFIG.                                               ORDCFGC\n" +
		"000300* Port of the metrics endpoint                                    ORDCFGC\n" +
		"000400     05  CFG-PORT  PIC 9(5).                                      ORDCFGC\n"
	roots, warns, err := Parse("ORDCFGC.cpy", src, Fixed)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Errorf("warnings: %v", warns)
	}
	rec := roots[0]
	if rec.Doc[0].Text != "@SERVICE ORDERS-BATCH  @PREFIX CFG-" || rec.Children[0].Doc[0].Text != "Port of the metrics endpoint" {
		t.Errorf("doc: %+v %+v", rec.Doc, rec.Children[0].Doc)
	}

	// An annotation that runs into column 73 is a warning.
	cut := "      *> Port " + strings.Repeat(".", 49) + " @max 655" + "35\n" +
		"       01  A.\n"
	_, warns, err = Parse("demo.cpy", cut, Fixed)
	if err != nil || len(warns) != 1 || warns[0].String() != `demo.cpy:1: warning: the annotation "@max 655" is cut at column 72 (cobc ignores columns 73-80); move it to the next comment line` {
		t.Errorf("got %v %v", warns, err)
	}
	_, warns, _ = Parse("demo.cpy", "      *> Port"+strings.Repeat(" ", 59)+"@max 1\n       01  A.\n", Fixed)
	if len(warns) != 1 || !strings.Contains(warns[0].Msg, `columns 73-80 hold "@max 1"`) {
		t.Errorf("got %v", warns)
	}
}

// A copybook of fields with no level-01 record gets a synthetic record.
func TestNoRecord(t *testing.T) {
	roots, _, err := Parse("f.cpy", "      *> Port\n           05  CFG-PORT PIC 9(5).\n           05  CFG-GRP.\n               10  CFG-X PIC X.\n", Fixed)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || !roots[0].Synthetic || len(roots[0].Children) != 2 || roots[0].Children[1].Children[0].Name != "CFG-X" {
		t.Fatalf("roots: %+v", roots[0])
	}
}

// An inline comment is marked, so its tags can be read.
func TestInlineComment(t *testing.T) {
	roots, _, err := Parse("f.cpy", "       01  R.\n           05  CFG-PORT PIC 9(5).  *> port number @default 80\n", Fixed)
	if err != nil {
		t.Fatal(err)
	}
	d := roots[0].Children[0].Doc
	if len(d) != 1 || !d[0].Inline || d[0].Text != "port number @default 80" {
		t.Fatalf("doc: %+v", d)
	}
}

func TestPictures(t *testing.T) {
	for pic, want := range map[string]Picture{
		"X(12)":    {Alpha: true, Size: 12},
		"XXX":      {Alpha: true, Size: 3},
		"9(5)":     {IntDigits: 5},
		"S9(18)":   {Signed: true, IntDigits: 18},
		"9V9(4)":   {IntDigits: 1, Frac: 4},
		"S9(3)V99": {Signed: true, IntDigits: 3, Frac: 2},
	} {
		got, err := ParsePicture(pic)
		if err != nil || got != want {
			t.Errorf("%s: got %+v %v, want %+v", pic, got, err, want)
		}
	}
	for _, pic := range []string{"Z(4)9", "X9", "9(40)", "S9V9V9"} {
		if _, err := ParsePicture(pic); err == nil {
			t.Errorf("%s: no error", pic)
		}
	}
}

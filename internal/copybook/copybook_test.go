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
	roots, err := Parse("demo.cpy", fixed, Fixed)
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
	got, err := Parse("demo.cpy", free.String(), Free)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Parse("demo.cpy", fixed, Fixed)
	if len(got[0].Children) != len(want[0].Children) || got[0].Children[1].Doc[0].Text != "@default info" {
		t.Fatalf("free format parsed differently: %+v", got[0])
	}
}

func TestFixedFormatErrors(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"      *> " + strings.Repeat("x", 70) + "\n       01  A.\n", "demo.cpy:1: comment runs past column 72"},
		{"01  DEMO-CONFIG.\n", "column 7 holds"},
		{"       01  A.\n      -    \"x\".\n", "continuation lines are not supported"},
		{"       01  A\n", "does not end with a period"},
		{"       05  A PIC X.\n", "is not under a level-01 record"},
	} {
		_, err := Parse("demo.cpy", c.src, Fixed)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: got %v, want %q", c.src, err, c.want)
		}
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

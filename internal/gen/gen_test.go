package gen

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docuconf/docuconf-cobol/internal/copybook"
)

var update = flag.Bool("update", false, "rewrite golden files")

func generate(t *testing.T, path string, opts Options) (cue, loader []byte) {
	t.Helper()
	c, err := Load(path, opts)
	if err != nil {
		t.Fatal(err)
	}
	cue, err = c.ContractCUE()
	if err != nil {
		t.Fatal(err)
	}
	loader, err = c.Loader()
	if err != nil {
		t.Fatal(err)
	}
	return cue, loader
}

func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Errorf("%s differs; run go test ./internal/gen -update and review the diff", path)
	}
}

// TestGolden covers every variable type and every file type.
func TestGolden(t *testing.T) {
	cue, loader := generate(t, "testdata/all-types.cpy", Options{})
	golden(t, "testdata/all-types.golden.cue", cue)
	golden(t, "testdata/GWCFG.golden.cbl", loader)
	for i, l := range strings.Split(string(loader), "\n") {
		if len(l) > 72 {
			t.Errorf("loader line %d passes column 72", i+1)
		}
	}
}

// TestExampleUpToDate fails when examples/orders needs regenerating.
func TestExampleUpToDate(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "orders")
	cue, loader := generate(t, filepath.Join(dir, "orders-config.cpy"), Options{})
	for name, got := range map[string][]byte{"contract.cue": cue, "ORDCFG.cbl": loader} {
		want, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(want) != string(got) {
			t.Errorf("examples/orders/%s is out of date; run docuconf-cobol generate examples/orders/orders-config.cpy", name)
		}
	}
}

// TestFreeFormat reads the same record written in free format.
func TestFreeFormat(t *testing.T) {
	src, err := os.ReadFile("testdata/all-types.cpy")
	if err != nil {
		t.Fatal(err)
	}
	var free strings.Builder
	for _, l := range strings.Split(string(src), "\n") {
		free.WriteString(strings.TrimLeft(l, " ") + "\n")
	}
	p := filepath.Join(t.TempDir(), "all-types.cpy")
	if err := os.WriteFile(p, []byte(free.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"schema-limits.json"} {
		data, _ := os.ReadFile(filepath.Join("testdata", f))
		os.WriteFile(filepath.Join(filepath.Dir(p), f), data, 0o644)
	}
	got, _ := generate(t, p, Options{Format: copybook.Free})
	want, _ := generate(t, "testdata/all-types.cpy", Options{})
	if string(got) != string(want) {
		t.Errorf("free format gives a different contract:\n%s", got)
	}
}

// TestLengthLimits checks the lengths a PIC X field gives url and json
// variables and string list items, and the contract's code point counts.
func TestLengthLimits(t *testing.T) {
	src := "      *> @service demo  @prefix CFG-\n       01  DEMO-CONFIG.\n" +
		"      *> Callback endpoint\n      *> @type url\n           05  CFG-CALLBACK PIC X(24).\n" +
		"      *> Run limits\n      *> @type json  @max-length 16\n           05  CFG-LIMITS PIC X(64).\n" +
		"      *> Branch codes\n      *> @item-min-length 2  @count CFG-N  @default ZÜ01 BE\n" +
		"           05  CFG-BRANCHES PIC X(6) OCCURS 4.\n           05  CFG-N PIC 9.\n"
	c, err := Build("demo.cpy", src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := c.ContractCUE()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"CALLBACK: {\n\t\t\ttype:        \"url\"",
		"maxLength:   24",
		"maxLength:   16",
		"itemMinLength: 2",
		"itemMaxLength: 6",
	} {
		if !strings.Contains(string(cue), want) {
			t.Errorf("contract lacks %q:\n%s", want, cue)
		}
	}
	// ZÜ01 is 4 characters, within the contract's itemMaxLength of 4, but
	// 5 bytes, more than PIC X(4) holds: the generator checks the bytes.
	_, err = Build("demo.cpy", strings.Replace(src, "PIC X(6) OCCURS", "PIC X(4) OCCURS", 1), Options{})
	if err == nil || !strings.Contains(err.Error(), `item "ZÜ01" is longer than PIC X(4) holds`) {
		t.Errorf("ZÜ01 in PIC X(4): %v", err)
	}
}

// TestCueVet checks the generated contracts against the meta-schema,
// with cue vet -c, as the platform will.
func TestCueVet(t *testing.T) {
	cue, err := exec.LookPath("cue")
	if err != nil {
		home, _ := os.UserHomeDir()
		cue = filepath.Join(home, "go", "bin", "cue")
		if _, err := os.Stat(cue); err != nil {
			t.Skip("cue not found; install cuelang.org/go/cmd/cue@v0.17.1")
		}
	}
	spec := os.Getenv("DOCUCONF_SPEC")
	if spec == "" {
		spec = filepath.Join("..", "..", "..", "docuconf-go", "spec", "cue")
	}
	if _, err := os.Stat(filepath.Join(spec, "contract")); err != nil {
		t.Skipf("docuconf spec not found at %s; set DOCUCONF_SPEC to docuconf-go/spec/cue", spec)
	}
	dir := t.TempDir()
	for _, sub := range []string{"cue.mod", "contract"} {
		if out, err := exec.Command("cp", "-r", filepath.Join(spec, sub), dir).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	all, _ := generate(t, "testdata/all-types.cpy", Options{})
	orders, _ := generate(t, "../../examples/orders/orders-config.cpy", Options{})
	for name, src := range map[string][]byte{"gateway": all, "orders": orders} {
		os.MkdirAll(filepath.Join(dir, name), 0o755)
		os.WriteFile(filepath.Join(dir, name, "contract.cue"), src, 0o644)
		cmd := exec.Command(cue, "vet", "-c", "./"+name)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("cue vet -c %s: %v\n%s", name, err, out)
		}
	}
}

// TestLoaderCompiles compiles the generated loader in fixed and free
// format.
func TestLoaderCompiles(t *testing.T) {
	cobc, err := exec.LookPath("cobc")
	if err != nil {
		t.Skip("cobc not found")
	}
	_, loader := generate(t, "testdata/all-types.cpy", Options{})
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "GWCFG.cbl"), loader, 0o644)
	abs, _ := filepath.Abs("testdata")
	for _, format := range []string{"-fixed", "-free"} {
		cmd := exec.Command(cobc, "-m", format, "-I", abs, "-o", filepath.Join(dir, "GWCFG.so"), "GWCFG.cbl")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("cobc %s: %v\n%s", format, err, out)
		}
	}
}

// TestProblems checks the messages a developer sees for mistakes.
func TestProblems(t *testing.T) {
	head := "      *> @service demo  @prefix CFG-\n       01  DEMO-CONFIG.\n"
	for _, c := range []struct{ name, body, want string }{
		{"no description", "           05  CFG-PORT PIC 9(5).\n",
			"demo.cpy:3: CFG-PORT: needs a description of at least 5 characters"},
		{"max beyond PIC", "      *> Port to listen on\n      *> @max 100000\n           05  CFG-PORT PIC 9(5).\n",
			"demo.cpy:4: CFG-PORT: @max 100000 is outside what PIC 9(5) holds (0 to 99999)"},
		{"negative min on unsigned", "      *> Offset to apply\n      *> @min -5\n           05  CFG-OFFSET PIC 9(3).\n",
			"PIC 9 is unsigned, use S9 for negative values"},
		{"default breaks a bound", "      *> Port to listen on\n      *> @min 1  @default 0\n           05  CFG-PORT PIC 9(5).\n",
			"demo.cpy:5: CFG-PORT (PORT): default 0 is below min 1"},
		{"unknown tag", "      *> Port to listen on\n      *> @maximum 5\n           05  CFG-PORT PIC 9(5).\n",
			"demo.cpy:4: CFG-PORT: unknown tag @maximum"},
		{"tag for another type", "      *> Port to listen on\n      *> @schemes https\n           05  CFG-PORT PIC 9(5).\n",
			"@schemes does not apply to a int variable"},
		{"list without count", "      *> Allowed origins\n           05  CFG-ORIGINS PIC X(40) OCCURS 4.\n",
			"a list needs a field for its number of items"},
		{"edited picture", "      *> Amount to charge\n           05  CFG-AMOUNT PIC ZZ9.99.\n",
			"PIC ZZ9.99 is an edited or unsupported picture"},
		{"enum value too long", "      *> Log level\n           05  CFG-LEVEL PIC X(4).\n               88  L-DEBUG VALUE \"debug\".\n",
			`enum value "debug" is longer than PIC X(4) holds`},
		{"bad env name", "      *> Port to listen on\n      *> @env LOG-LEVEL\n           05  CFG-PORT PIC 9(5).\n",
			"@env LOG-LEVEL is not an environment variable name"},
		{"duration unit", "      *> Request timeout\n      *> @type duration\n           05  CFG-TIMEOUT PIC 9(5).\n",
			"@unit must be one of ns, us, ms, s, m, h"},
		{"default finer than unit", "      *> Request timeout\n      *> @unit s  @default 1500ms\n           05  CFG-TIMEOUT PIC 9(5).\n",
			"1500ms is not a whole number of s"},
		{"file without path", "      *> Orders to read\n      *> @file orders\n           05  CFG-ORDERS PIC X(100).\n",
			"a file input needs @path"},
		{"item lengths on ints", "      *> Shards to own\n      *> @item-max-length 3  @count CFG-N\n           05  CFG-SHARDS PIC 9(4) OCCURS 4.\n           05  CFG-N PIC 9.\n",
			"@item-max-length does not apply to a list variable"},
		{"item max length beyond PIC", "      *> Branch codes\n      *> @item-max-length 5  @count CFG-N\n           05  CFG-BRANCHES PIC X(4) OCCURS 4.\n           05  CFG-N PIC 9.\n",
			"@item-max-length 5 is more than PIC X(4) holds"},
		{"item min length above max", "      *> Branch codes\n      *> @item-min-length 3  @item-max-length 2\n      *> @count CFG-N\n           05  CFG-BRANCHES PIC X(4) OCCURS 4.\n           05  CFG-N PIC 9.\n",
			"@item-min-length 3 is above the item maximum length 2"},
		{"string tags on a list", "      *> Branch codes\n      *> @max-length 2  @count CFG-N\n           05  CFG-BRANCHES PIC X(4) OCCURS 4.\n           05  CFG-N PIC 9.\n",
			"@max-length does not apply to a list variable"},
		{"url max length beyond PIC", "      *> Callback endpoint\n      *> @type url  @max-length 50\n           05  CFG-CALLBACK PIC X(40).\n",
			"@max-length 50 is more than PIC X(40) holds"},
		{"item default too long", "      *> Branch codes\n      *> @item-max-length 2  @count CFG-N  @default BE GENEVA\n           05  CFG-BRANCHES PIC X(8) OCCURS 4.\n           05  CFG-N PIC 9.\n",
			"itemMaxLength 2"},
		{"json default too long", "      *> Run limits\n      *> @type json  @max-length 8\n      *> @default \"{\"\"n\"\": \"\"abcdef\"\"}\"\n           05  CFG-LIMITS PIC X(64).\n",
			"maxLength 8"},
		{"duplicate variable", "      *> Port to listen on\n           05  CFG-PORT PIC 9(5).\n      *> Port again here\n      *> @env PORT\n           05  CFG-PORT2 PIC 9(5).\n",
			"variable PORT is also read by the field at line 4"},
	} {
		_, err := Build("demo.cpy", head+c.body, Options{})
		if err == nil {
			c2, _ := Build("demo.cpy", head+c.body, Options{})
			_, err = c2.ContractCUE()
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v\nwant %q", c.name, err, c.want)
		}
	}
	if _, err := Build("demo.cpy", "       01  DEMO-CONFIG.\n      *> Port to listen on\n           05  CFG-PORT PIC 9(5).\n", Options{}); err == nil ||
		!strings.Contains(err.Error(), "name the service") {
		t.Errorf("no service: %v", err)
	}
}

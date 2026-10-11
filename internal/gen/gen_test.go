package gen

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/docuconf/docuconf-cobol/copybooks"
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

// generatorVersion matches the value of metadata.generator.version. It is
// Version, which every release PR bumps, so golden comparisons ignore it.
var generatorVersion = regexp.MustCompile(`(generator:\s*\{[^{}]*?\bversion:\s*)"[^"]*"`)

func withoutGeneratorVersion(b []byte) string {
	return generatorVersion.ReplaceAllString(string(b), `${1}"<generator-version>"`)
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
	if withoutGeneratorVersion(want) != withoutGeneratorVersion(got) {
		t.Errorf("%s differs; run go test ./internal/gen -update and review the diff", path)
	}
}

func TestGoldenComparisonIgnoresOnlyTheGeneratorVersion(t *testing.T) {
	cue, _ := generate(t, "testdata/all-types.cpy", Options{})
	bumped := strings.Replace(string(cue), `"`+Version+`"`, `"99.0.0"`, 1)
	if bumped == string(cue) || withoutGeneratorVersion([]byte(bumped)) != withoutGeneratorVersion(cue) {
		t.Error("a different generator version must compare equal")
	}
	renamed := strings.Replace(string(cue), `"docuconf-cobol"`, `"other"`, 1)
	if renamed == string(cue) || withoutGeneratorVersion([]byte(renamed)) == withoutGeneratorVersion(cue) {
		t.Error("any other difference must still fail")
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
		if err != nil || withoutGeneratorVersion(want) != withoutGeneratorVersion(got) {
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

	// -runtime copy: the loader COPYs the runtime from a copy library.
	_, loader = generate(t, "testdata/all-types.cpy", Options{Runtime: "copy"})
	lib := t.TempDir()
	os.WriteFile(filepath.Join(lib, "DCRTWS.cpy"), []byte(copybooks.WorkingStorage), 0o644)
	os.WriteFile(filepath.Join(lib, "DCRTPD.cpy"), []byte(copybooks.Procedures), 0o644)
	os.WriteFile(filepath.Join(dir, "GWCFG.cbl"), loader, 0o644)
	cmd := exec.Command(cobc, "-m", "-I", abs, "-I", lib, "-o", filepath.Join(dir, "GWCFG.so"), "GWCFG.cbl")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("cobc -runtime copy: %v\n%s", err, out)
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
		{"reload watch", "      *> Orders to read\n      *> @file orders  @path /data/orders.txt  @reload watch\n           05  CFG-ORDERS PIC X(100).\n",
			"CFG-ORDERS: @reload watch needs the program to reload the file itself; a COBOL loader reads paths once, so use restart"},
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
		{"max beyond PIC after min", "      *> Port to listen on\n      *> @min 1  @max 100000\n           05  CFG-PORT PIC 9(5).\n",
			"@max 100000 is outside what PIC 9(5) holds (0 to 99999)"},
		{"did you mean", "      *> Port to listen on\n      *> @defualt 80\n           05  CFG-PORT PIC 9(5).\n",
			"unknown tag @defualt; did you mean @default?"},
		{"secret with default", "      *> Database password\n      *> @secret  @default x\n           05  CFG-PWD PIC X(20).\n",
			"demo.cpy:4: CFG-PWD: a @secret variable cannot have a default (@default); supply it from a Kubernetes Secret"},
		{"required with VALUE", "      *> Database host\n      *> @required\n           05  CFG-HOST PIC X(20) VALUE 'db'.\n",
			"demo.cpy:5: CFG-HOST: a @required variable cannot have a default (VALUE)"},
		{"VALUE and default disagree", "      *> Port to listen on\n      *> @default 80\n           05  CFG-PORT PIC 9(5) VALUE 8080.\n",
			"CFG-PORT: VALUE 8080 and @default 80 disagree; keep one of them"},
		{"VALUE on a table", "      *> Allowed origins\n      *> @count CFG-N\n           05  CFG-ORIGINS PIC X(40) OCCURS 4 VALUE 'x'.\n           05  CFG-N PIC 9.\n",
			"VALUE x on an OCCURS table sets every item; give the list's default with @default"},
		{"default too precise", "      *> Share to sample\n      *> @default 0.123\n           05  CFG-R PIC 9V99.\n",
			"CFG-R: @default: 0.123 has more decimal places than PIC 9V99 holds"},
		{"VALUE too precise", "      *> Share to sample\n           05  CFG-R PIC 9V99 VALUE 0.125.\n",
			"CFG-R: VALUE: 0.125 has more decimal places than PIC 9V99 holds"},
		{"min too precise", "      *> Share to sample\n      *> @min 0.125\n           05  CFG-R PIC 9V99.\n",
			"@min 0.125 has more decimal places than PIC 9V99 holds"},
		{"int default not an integer", "      *> Port to listen on\n      *> @default 1.5\n           05  CFG-PORT PIC 9(5).\n",
			`"1.5" is not an integer`},
		{"collides with the loader", "      *> Name of the job\n      *> @env JOB_NAME\n           05  DC-NAME PIC X(20).\n",
			"demo.cpy:5: DC-NAME is a name in the loader's working storage (DCRTWS); rename it"},
		{"pattern not RE2", "      *> Region code\n      *> @pattern \"(?=x)\"\n           05  CFG-REGION PIC X(20).\n",
			`@pattern "(?=x)" is not an RE2 pattern`},
		{"key set without a table", "      *> Webhook keys\n      *> @type keySet\n           05  CFG-KEYS PIC X(40).\n",
			"a keySet is a table of keys: PIC X(n) OCCURS m, with @count"},
		{"key set with a default", "      *> Webhook keys\n      *> @type keySet  @count CFG-N  @default abc\n           05  CFG-KEYS PIC X(40) OCCURS 2.\n           05  CFG-N PIC 9.\n",
			"a @secret variable cannot have a default"},
		{"key set with no keys", "      *> Webhook keys\n      *> @type keySet  @count CFG-N  @min-keys 0\n           05  CFG-KEYS PIC X(40) OCCURS 2.\n           05  CFG-N PIC 9.\n",
			"@min-keys must be at least 1"},
		{"list tags on a key set", "      *> Webhook keys\n      *> @type keySet  @count CFG-N  @min-items 1\n           05  CFG-KEYS PIC X(40) OCCURS 2.\n           05  CFG-N PIC 9.\n",
			"@min-items does not apply to a keySet variable"},
		{"key max length beyond PIC", "      *> Webhook keys\n      *> @type keySet  @count CFG-N  @key-max-length 50\n           05  CFG-KEYS PIC X(40) OCCURS 2.\n           05  CFG-N PIC 9.\n",
			"@key-max-length 50 is more than PIC X(40) holds"},
		{"key set of ints", "      *> Webhook keys\n      *> @type keySet  @count CFG-N\n           05  CFG-KEYS PIC 9(4) OCCURS 2.\n           05  CFG-N PIC 9.\n",
			"a keySet is a table of keys"},
		{"required and deprecated", "      *> Old port to listen on\n      *> @required  @deprecated \"Use PORT instead\"\n           05  CFG-OLD-PORT PIC 9(5).\n",
			"CFG-OLD-PORT: a @required variable cannot be @deprecated"},
		{"blank deprecation", "      *> Old port to listen on\n      *> @deprecated \" \"\n           05  CFG-OLD-PORT PIC 9(5).\n",
			"@deprecated must say what to use instead, or why the variable is going away"},
		{"replaced-by without deprecated", "      *> Old port to listen on\n      *> @replaced-by PORT\n           05  CFG-OLD-PORT PIC 9(5).\n",
			"@replaced-by needs @deprecated"},
		{"lines in order", "      *> Pw\n      *> @secret  @default x\n           05  CFG-PWD PIC X(20).\n",
			"demo.cpy:4: CFG-PWD: a @secret variable cannot have a default (@default); supply it from a Kubernetes Secret\ndemo.cpy:5: CFG-PWD: needs a description"},
	} {
		_, err := Build("demo.cpy", head+c.body, Options{})
		if err == nil {
			c2, _ := Build("demo.cpy", head+c.body, Options{})
			if _, err = c2.ContractCUE(); err == nil {
				_, err = c2.Loader()
			}
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

// Tags may follow text in an inline comment, and Y, N, 1 and 0 are bool
// defaults.
func TestInlineTagsAndBoolDefaults(t *testing.T) {
	src := "      *> @service demo\n       01  DEMO-CONFIG.\n" +
		"           05  CFG-PORT PIC 9(5).  *> port number @default 80\n" +
		"      *> Verbose output, mailed to ops@example.com\n      *> @type bool  @default Y\n           05  CFG-VERBOSE PIC X.\n"
	c, err := Build("demo.cpy", src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := c.ContractJSON()
	for _, w := range []string{`"default": 80`, `"description": "port number"`, `"default": true`, `ops@example.com`} {
		if !strings.Contains(string(doc), w) {
			t.Errorf("want %s in:\n%s", w, doc)
		}
	}
}

// A copybook with several records needs -record; one with none is
// wrapped, and its record tags come from above the first field.
func TestRecords(t *testing.T) {
	two := "      *> @service a\n       01  A-CONFIG.\n      *> Port to listen on\n           05  A-PORT PIC 9(5).\n" +
		"      *> @service b\n       01  B-CONFIG.\n      *> Port to listen on\n           05  B-PORT PIC 9(5).\n"
	if _, err := Build("two.cpy", two, Options{}); err == nil || !strings.Contains(err.Error(), "the copybook has 2 level-01 records (A-CONFIG, B-CONFIG); pick the configuration record with -record A-CONFIG") {
		t.Errorf("got %v", err)
	}
	c, err := Build("two.cpy", two, Options{Record: "b-config"})
	if err != nil || c.Service != "b" || c.Vars[0].Env != "B_PORT" {
		t.Fatalf("got %+v %v", c, err)
	}
	if _, err := Build("two.cpy", two, Options{Record: "C-CONFIG"}); err == nil || !strings.Contains(err.Error(), "the level-01 records are A-CONFIG, B-CONFIG") {
		t.Errorf("got %v", err)
	}

	fields := "      *> @service demo  @prefix CFG-\n      *> Port to listen on\n      *> @min 1\n           05  CFG-PORT PIC 9(5).\n"
	c, err = Build("CFGFLDS.cpy", fields, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Wrap || c.Record != "DC-RECORD" || c.Service != "demo" || c.Program != "CFGFLDSL" || c.Vars[0].Env != "PORT" {
		t.Fatalf("got %+v", c)
	}
	loader, err := c.Loader()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "       01  DC-RECORD.\n       COPY CFGFLDS.\n       PROCEDURE DIVISION USING DC-RECORD.\n") {
		t.Errorf("loader linkage:\n%s", loader)
	}
	c, _ = Build("CFGFLDS.cpy", fields, Options{Record: "WS-CFG", Runtime: "copy"})
	loader, _ = c.Loader()
	if !strings.Contains(string(loader), "01  WS-CFG.") || !strings.Contains(string(loader), "       COPY DCRTWS.\n") || !strings.Contains(string(loader), "       COPY DCRTPD.\n") {
		t.Errorf("loader:\n%s", loader)
	}
}

// The first paragraph of the comment is the description, and the rest
// the details (SPEC §4.2): CommonMark, written as is, with indentation
// kept, after the description in the contract.
func TestDetails(t *testing.T) {
	src := "      *> @service demo  @prefix CFG-\n       01  DEMO-CONFIG.\n" +
		"      *> Number of workers that share the input\n" +
		"      *> on this node.\n" +
		"      *>\n" +
		"      *> Each worker holds a database connection, so keep it\n" +
		"      *> at or below the pool size:\n" +
		"      *>\n" +
		"      *> - one connection per worker;\n" +
		"      *>   - nested, *indented*;\n" +
		"      *> - plus one for migrations.\n" +
		"      *> ------------------------------------------------\n" +
		"      *> ```sh\n" +
		"      *> @min stays text inside a code block\n" +
		"      *>   kubectl scale --replicas=2\n" +
		"      *> ```\n" +
		"      *>\n" +
		"      *> # Grüße aus 東京\n" +
		"      *> @min 1  @max 64  @default 4\n" +
		"           05  CFG-WORKER-COUNT PIC 9(2).\n" +
		"      *> HTTP listen port\n" +
		"      *> @details \"Behind the mesh, keep the **default**.\"\n" +
		"           05  CFG-PORT PIC 9(5).\n" +
		"      *> Log level, one paragraph\n" +
		"      *>\n" +
		"           05  CFG-LEVEL PIC X(5).\n" +
		"      *> The orders to summarise\n" +
		"      *>\n" +
		"      *> One order per line, as `id,amount`.\n" +
		"      *> @file orders text  @path /data/orders.txt\n" +
		"           05  CFG-ORDERS-PATH PIC X(256).\n"
	c, err := Build("demo.cpy", src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := c.ContractCUE()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"description: \"Number of workers that share the input on this node.\"\n\t\t\tdetails:     \"Each worker holds a database connection, so keep it\\nat or below the pool size:\\n\\n- one connection per worker;\\n  - nested, *indented*;\\n- plus one for migrations.\\n```sh\\n@min stays text inside a code block\\n  kubectl scale --replicas=2\\n```\\n\\n# Grüße aus 東京\"\n",
		"description: \"HTTP listen port\"\n\t\t\tdetails:     \"Behind the mesh, keep the **default**.\"\n",
		"description: \"The orders to summarise\"\n\t\t\tdetails:     \"One order per line, as `id,amount`.\"\n",
		"description: \"Log level, one paragraph\"\n\t\t\tmaxLength:",
	} {
		if !strings.Contains(string(cue), want) {
			t.Errorf("contract lacks %q:\n%s", want, cue)
		}
	}

	// Details over 4000 characters, or blank, fail like a short
	// description; 4000 characters pass.
	long := func(lines int, extra string) string {
		s := "      *> Number of workers\n      *>\n"
		for i := 0; i < lines; i++ {
			s += "      *> " + strings.Repeat("日本", 10) + "\n" // 20 characters a line
		}
		return s + "      *> " + extra + "\n           05  CFG-WORKERS PIC 9(2).\n"
	}
	head := "      *> @service demo  @prefix CFG-\n       01  DEMO-CONFIG.\n"
	// 190 lines of 20 characters, 190 newlines and 10 characters: 4000.
	if _, err := Build("demo.cpy", head+long(190, strings.Repeat("日本", 5)), Options{}); err != nil {
		t.Errorf("4000 characters: %v", err)
	}
	_, err = Build("demo.cpy", head+long(190, strings.Repeat("日本", 5)+"日"), Options{})
	if err == nil || !strings.Contains(err.Error(), "demo.cpy:196: CFG-WORKERS: details are 4001 characters (the comment after its first paragraph, or @details); details may have at most 4000") {
		t.Errorf("4001 characters: %v", err)
	}
	_, err = Build("demo.cpy", head+"      *> Number of workers\n      *> @details \"  \"\n           05  CFG-WORKERS PIC 9(2).\n", Options{})
	if err == nil || !strings.Contains(err.Error(), "CFG-WORKERS: @details must not be blank") {
		t.Errorf("blank details: %v", err)
	}
	_, err = Build("demo.cpy", head+"      *>\n      *> Each worker holds a connection.\n           05  CFG-WORKERS PIC 9(2).\n", Options{})
	if err != nil {
		t.Errorf("a leading empty line: %v", err)
	}
	_, err = Build("demo.cpy", head+"      *> @details \"Without a description.\"\n           05  CFG-WORKERS PIC 9(2).\n", Options{})
	if err == nil || !strings.Contains(err.Error(), "CFG-WORKERS: needs a description") {
		t.Errorf("details without a description: %v", err)
	}
}

// A pattern too large for the loader's tables is left to docuconf exec,
// with a warning; one that fits is compiled into the loader.
func TestPatternTables(t *testing.T) {
	src := "      *> @service demo\n       01  DEMO-CONFIG.\n" +
		"      *> Region code\n      *> @pattern \"^[a-z]{2}-[a-z]+-[0-9]$\"\n           05  CFG-REGION PIC X(20).\n" +
		"      *> Free text\n      *> @pattern \"^.{1,500}$\"\n           05  CFG-NOTE PIC X(600).\n"
	c, err := Build("demo.cpy", src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0], "CFG-NOTE: @pattern compiles to more than the loader's 1000 instructions") {
		t.Errorf("warnings: %q", c.Warnings)
	}
	if c.Vars[0].Pattern == nil || c.Vars[1].Pattern != nil {
		t.Errorf("patterns: %v, %v", c.Vars[0].Pattern, c.Vars[1].Pattern)
	}
	loader, err := c.Loader()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(loader), "PERFORM DC-RX-MATCH"); n != 1 {
		t.Errorf("the loader runs %d patterns, want 1", n)
	}
	// Case folding adds every equivalent: (?i)k also matches the Kelvin sign.
	p, err := compileRE2("(?i)k")
	if err != nil || p == nil || !strings.Contains(p.ranges, "00000750000075") || !strings.Contains(p.ranges, "00084900008490") {
		t.Errorf("(?i)k: %+v %v", p, err)
	}
}

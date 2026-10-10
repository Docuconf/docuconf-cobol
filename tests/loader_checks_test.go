package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/docuconf/docuconf-cobol/internal/gen"
)

// The loader checks every variable rule itself, so a program run
// without docuconf exec still rejects what exec would: these tests
// compare the loader with Go (patterns with regexp, JSON with
// encoding/json, URLs with the contract's URL form).

// checksProgram generates the loader for the copybook body (the fields
// of record CHK-CONFIG) and compiles a program that only CALLs it.
func checksProgram(t *testing.T, fields string) string {
	t.Helper()
	cobc, err := exec.LookPath("cobc")
	if err != nil {
		skipOrFail(t, "cobc (GnuCOBOL) not found")
	}
	dir := t.TempDir()
	cpy := "      *> @service checks  @program CHKCFG\n       01  CHK-CONFIG.\n" + fields
	if err := os.WriteFile(filepath.Join(dir, "chk.cpy"), []byte(cpy), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := gen.Load(filepath.Join(dir, "chk.cpy"), gen.Options{})
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, cpy)
	}
	loader, err := c.Loader()
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "CHKCFG.cbl"), loader, 0o644)
	os.WriteFile(filepath.Join(dir, "main.cbl"), []byte(`       IDENTIFICATION DIVISION.
       PROGRAM-ID. MAIN.
       DATA DIVISION.
       WORKING-STORAGE SECTION.
       COPY "chk.cpy".
       PROCEDURE DIVISION.
           CALL "CHKCFG" USING CHK-CONFIG
           STOP RUN.
`), 0o644)
	cmd := exec.Command(cobc, "-x", "-o", "main", "main.cbl", "CHKCFG.cbl")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cobc: %v\n%s", err, out)
	}
	return filepath.Join(dir, "main")
}

// problems runs bin with exactly env and returns its problem lines.
func problems(t *testing.T, bin string, env map[string]string) string {
	t.Helper()
	list := []string{"DOCUCONF_TERMINATION_LOG=-"}
	for k, v := range env {
		list = append(list, k+"="+v)
	}
	_, stderr := runLoader(t, bin, list...)
	return stderr
}

func TestLoaderPatterns(t *testing.T) {
	patterns := []string{
		`^[a-z][a-z0-9-]*$`, `[0-9]{3}`, `(?i)^hello`, `\bcat\b`, `\Bcat`, `(?m)^b$`, `^a.c$`, `(?s)^a.c$`,
		`^\p{Greek}+$`, `^(foo|bar)+$`, `^x{2,3}$`, `^[^a-c]+$`, `colou?r`, `^\d+(\.\d+)?$`,
		`^[\w.+-]+@[\w-]+\.[a-z]{2,}$`, `日本`, `^.{3}$`, `(?i)straße`, `a+?b`, `\Aabc\z`, `x*`, `^$`,
		`^[[:upper:]]+$`, `(?i)[k]`, `\s`, `^(a|ab)(c|bcd)(d*)$`,
	}
	inputs := []string{
		"abc", "hello world", "HeLLo", "a cat sat", "concat", "a\nb\nc", "a\nc", "αβγ", "foobarfoo", "xx", "xxxx",
		"ABC", "color", "colour", "", "12.5", "12.", "a.b+c@ex-ample.com", "日本語", "ab\xffc", "STRASSE", "STRAẞE",
		"aab", "x123y", "slug-1", "Slug", "\u212a", "tab\there", "abcd", "日本x",
	}
	var cpy strings.Builder
	for i, p := range patterns {
		fmt.Fprintf(&cpy, "      *> Pattern number %d\n      *> @env P%d  @pattern %s\n           05  P-%d PIC X(64).\n",
			i, i, quote(p), i)
	}
	bin := checksProgram(t, cpy.String())
	for _, in := range inputs {
		env := map[string]string{}
		for i := range patterns {
			env[fmt.Sprintf("P%d", i)] = in
		}
		out := problems(t, bin, env)
		for i, p := range patterns {
			want := regexp.MustCompile(p).MatchString(in)
			got := !hasViolation(out, fmt.Sprintf("P%d", i), "pattern_mismatch")
			if got != want {
				t.Errorf("%q on %q: the loader says match=%v, Go says %v", p, in, got, want)
			}
		}
	}
}

func TestLoaderJSONSyntax(t *testing.T) {
	bin := checksProgram(t, "      *> A JSON document\n      *> @env DOC  @type json\n           05  J-DOC PIC X(512).\n")
	for _, in := range []string{
		`{}`, `[]`, `[1,2]`, `{"a":[1,{"b":null}],"c":true}`, `"str"`, `1e5`, `-0.5`, `0`, `01`, `[1,]`, `{"a"}`,
		`{a:1}`, `tru`, `nul`, `"\u00e9"`, `"\x"`, "\"tab\there\"", `[1] 2`, ` {"a" : 1 }`, `1.`, `.5`, `-`,
		`[[[[]]]]`, `{"a":1,}`, `"unterminated`, `true false`, `"\ud83d\ude00"`, `{"a":1}}`, `1E+2`, `1e`, `-01`,
		`{"a":1 "b":2}`, `[` + strings.Repeat("[", 300) + `]`, `"\u12"`, `false`, `null`, `{"":""}`,
	} {
		out := problems(t, bin, map[string]string{"DOC": in})
		got := !hasViolation(out, "DOC", "invalid_type")
		if want := json.Valid([]byte(in)); got != want && !(strings.Count(in, "[") > 256) {
			t.Errorf("%q: the loader says valid=%v, Go says %v\n%s", in, got, want, out)
		}
	}
}

func TestLoaderURLs(t *testing.T) {
	form := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://[^\s]+$`)
	bin := checksProgram(t, `      *> Upstream base URL
      *> @env BASE  @type url  @schemes https wss
           05  U-BASE PIC X(128).
      *> Any URL
      *> @env ANY  @type url
           05  U-ANY PIC X(128).
`)
	for _, in := range []string{
		"https://x", "HTTPS://x", "wss://h:1/p?q", "http://x", "1http://x", "ht tp://x", "https://", "https://a b",
		"https:/x", "a+b.c-d://x", "x://y", "://x", "https://\tx", "mailto:x@y", "https://例え.jp",
	} {
		out := problems(t, bin, map[string]string{"BASE": in, "ANY": in})
		valid := form.MatchString(in)
		if got := !hasViolation(out, "ANY", "invalid_type"); got != valid {
			t.Errorf("ANY=%q: the loader says valid=%v, want %v\n%s", in, got, valid, out)
		}
		scheme := strings.ToLower(strings.SplitN(in, "://", 2)[0])
		allowed := scheme == "https" || scheme == "wss"
		switch {
		case !valid && !hasViolation(out, "BASE", "invalid_type"):
			t.Errorf("BASE=%q: want invalid_type\n%s", in, out)
		case valid && !allowed && !hasViolation(out, "BASE", "invalid_scheme"):
			t.Errorf("BASE=%q: want invalid_scheme\n%s", in, out)
		case valid && allowed && strings.Contains(out, "BASE:"):
			t.Errorf("BASE=%q: want no problem\n%s", in, out)
		}
	}
}

func TestLoaderLengthsAndSecrets(t *testing.T) {
	bin := checksProgram(t, `      *> A short name, in characters
      *> @env NAME  @min-length 2  @max-length 4
           05  L-NAME PIC X(32).
      *> Branch codes
      *> @env CODES  @item-min-length 2  @item-max-length 4
      *> @count L-CODE-N
           05  L-CODES PIC X(16) OCCURS 5.
           05  L-CODE-N PIC 9.
      *> A secret token
      *> @env TOKEN  @secret  @min-length 8
           05  L-TOKEN PIC X(64).
      *> Secret keys
      *> @env KEYS  @secret  @count L-KEY-N
           05  L-KEYS PIC X(64) OCCURS 3.
           05  L-KEY-N PIC 9.
      *> Secret keys, one variable each
      *> @env IKEYS  @secret  @encoding indexed  @count L-IKEY-N
           05  L-IKEYS PIC X(64) OCCURS 3.
           05  L-IKEY-N PIC 9.
      *> A JSON limit
      *> @env LIMIT  @type json  @max-length 10
           05  L-LIMIT PIC X(64).
`)
	for _, c := range []struct {
		env  map[string]string
		want []string // NAME/code; nil for no problem
	}{
		{map[string]string{"NAME": "日本", "CODES": "ZÜ01,BE", "TOKEN": "tok-12345", "KEYS": "a,b", "IKEYS__0": "a", "LIMIT": `{"n":"日本"}`}, nil},
		{map[string]string{"NAME": "日本語の道"}, []string{"NAME/out_of_range"}},
		{map[string]string{"NAME": "日"}, []string{"NAME/out_of_range"}},
		{map[string]string{"NAME": ""}, []string{"NAME/out_of_range"}},
		{map[string]string{"CODES": "BE,B"}, []string{"CODES/out_of_range"}},
		{map[string]string{"CODES": "BE,ZÜRICH"}, []string{"CODES/out_of_range"}},
		{map[string]string{"CODES": "BE,"}, []string{"CODES/out_of_range"}},
		{map[string]string{"TOKEN": "s3cr3t"}, []string{"TOKEN/out_of_range"}},
		{map[string]string{"TOKEN": "vault:secret/data/app#token"}, []string{"TOKEN/invalid_type"}},
		{map[string]string{"KEYS": "op://vault/item/field,b"}, []string{"KEYS/invalid_type"}},
		{map[string]string{"IKEYS__0": "a", "IKEYS__1": "ref+awssm://x"}, []string{"IKEYS/invalid_type"}},
		{map[string]string{"IKEYS__0": "a", "IKEYS__2": "c"}, []string{"IKEYS/invalid_type"}},
		{map[string]string{"IKEYS__1": "b"}, []string{"IKEYS/invalid_type"}},
		{map[string]string{"LIMIT": `{"n": 1234}`}, []string{"LIMIT/out_of_range"}},
	} {
		out := problems(t, bin, c.env)
		var got []string
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			if m := regexp.MustCompile(`^  ([A-Z_]+): .*\(([a-z_]+)\)$`).FindStringSubmatch(l); m != nil {
				got = append(got, m[1]+"/"+m[2])
			}
		}
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Errorf("%v: got %v, want %v\n%s", c.env, got, c.want, out)
		}
		for _, secret := range []string{"tok-12345", "s3cr3t", "vault:secret/data/app#token", "op://vault/item/field", "ref+awssm://x"} {
			if strings.Contains(out, secret) {
				t.Errorf("%v: the loader printed a secret value\n%s", c.env, out)
			}
		}
	}
}

// TestLoaderEmptyKey checks the message SPEC section 4.3 words exactly:
// "key N is empty", N the key's 1-based position as received.
func TestLoaderEmptyKey(t *testing.T) {
	bin := checksProgram(t, `      *> Webhook keys
      *> @env KEYS  @type keySet  @count E-KEY-N  @key-min-length 3
           05  E-KEYS PIC X(16) OCCURS 3.
           05  E-KEY-N PIC 9.
`)
	for in, want := range map[string]string{
		"old,":     "  KEYS: key 2 is empty (out_of_range)",
		",new":     "  KEYS: key 1 is empty (out_of_range)",
		"abc,,def": "  KEYS: key 2 is empty (out_of_range)",
	} {
		out := problems(t, bin, map[string]string{"KEYS": in})
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if !strings.Contains(out, want+"\n") || len(lines) != 2 {
			t.Errorf("KEYS=%q: want only %q\n%s", in, want, out)
		}
	}
}

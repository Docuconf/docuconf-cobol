package tests

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docuconf/docuconf-cobol/internal/gen"
)

// loaderProgram compiles testdata/loader.cpy's loader with a program
// that prints every field, to check the loader on its own, without
// docuconf exec in front of it.
func loaderProgram(t *testing.T) string {
	t.Helper()
	cobc, err := exec.LookPath("cobc")
	if err != nil {
		skipOrFail(t, "cobc (GnuCOBOL) not found")
	}
	dir := t.TempDir()
	src, err := os.ReadFile("testdata/loader.cpy")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "loader.cpy"), src, 0o644)
	c, err := gen.Load(filepath.Join(dir, "loader.cpy"), gen.Options{})
	if err != nil {
		t.Fatal(err)
	}
	loader, err := c.Loader()
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "TESTCFG.cbl"), loader, 0o644)
	os.WriteFile(filepath.Join(dir, "main.cbl"), []byte(`       IDENTIFICATION DIVISION.
       PROGRAM-ID. MAIN.
       DATA DIVISION.
       WORKING-STORAGE SECTION.
       COPY "loader.cpy".
       01  K PIC 9.
       PROCEDURE DIVISION.
           CALL "TESTCFG" USING TEST-CONFIG
           DISPLAY "RC=" RETURN-CODE END-DISPLAY
           DISPLAY "NAME=" T-NAME END-DISPLAY
           DISPLAY "COUNT=" T-COUNT END-DISPLAY
           DISPLAY "OFFSET=" T-OFFSET " " T-OFFSET-SET END-DISPLAY
           DISPLAY "RATIO=" T-RATIO END-DISPLAY
           DISPLAY "TIMEOUT=" T-TIMEOUT END-DISPLAY
           DISPLAY "RETRY=" T-RETRY END-DISPLAY
           DISPLAY "VERBOSE=" T-VERBOSE END-DISPLAY
           DISPLAY "SHARDS=" T-SHARD-COUNT END-DISPLAY
           PERFORM VARYING K FROM 1 BY 1 UNTIL K > T-SHARD-COUNT
               DISPLAY "SHARD=" T-SHARDS(K) END-DISPLAY
           END-PERFORM
           DISPLAY "TAGS=" T-TAG-COUNT END-DISPLAY
           PERFORM VARYING K FROM 1 BY 1 UNTIL K > T-TAG-COUNT
               DISPLAY "TAG=" T-TAGS(K) END-DISPLAY
           END-PERFORM
           DISPLAY "REGION=" T-REGION END-DISPLAY
           DISPLAY "ORDERS=" FUNCTION TRIM(T-ORDERS-PATH) END-DISPLAY
           STOP RUN.
`), 0o644)
	cmd := exec.Command(cobc, "-x", "-o", "main", "main.cbl", "TESTCFG.cbl")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cobc: %v\n%s", err, out)
	}
	return filepath.Join(dir, "main")
}

func runLoader(t *testing.T, bin string, env ...string) (stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = env
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatal(err)
		}
	}
	return o.String(), e.String()
}

func TestLoaderValues(t *testing.T) {
	bin := loaderProgram(t)
	out, errOut := runLoader(t, bin,
		"T_NAME=", "REGION=eu", "NAME=héll", "OFFSET=-12", "RATIO=0.5", "TIMEOUT=1m2s",
		"RETRY=1.00:00:30", "VERBOSE=TRUE", "SHARDS=[1, -2,3]", "TAGS=a;b c",
		"ORDERS_FILE=/in/orders.txt", "DOCUCONF_FILE_ROOT=/tmp/root/")
	want := []string{
		"RC=+000000000", "NAME=héll", "COUNT=007", "OFFSET=-0012 Y", "RATIO=0.50",
		"TIMEOUT=0062000", "RETRY=086430", "VERBOSE=1", "SHARDS=3", "SHARD=+0001",
		"SHARD=-0002", "SHARD=+0003", "TAGS=2", "TAG=a       ", "TAG=b c     ", "REGION=eu",
		"ORDERS=/tmp/root/in/orders.txt",
	}
	if errOut != "" || strings.Join(strings.Split(strings.TrimSpace(out), "\n"), "|") != strings.Join(want, "|") {
		t.Fatalf("stdout:\n%s\nstderr:\n%s\nwant:\n%s", out, errOut, strings.Join(want, "\n"))
	}

	// Defaults, an unset optional, and the default path.
	out, errOut = runLoader(t, bin, "REGION=us")
	for _, w := range []string{"RC=+000000000", "COUNT=007", "OFFSET=+0000 N", "TIMEOUT=0030000", "SHARDS=0", "ORDERS=/data/orders.txt"} {
		if !strings.Contains(out, w+"\n") {
			t.Errorf("want %s in:\n%s%s", w, out, errOut)
		}
	}
}

// TestLoaderProblems covers what docuconf exec cannot see: a value that
// is valid for the contract but does not fit the COBOL field.
func TestLoaderProblems(t *testing.T) {
	bin := loaderProgram(t)
	out, errOut := runLoader(t, bin,
		"NAME=héllo", "TOKEN=hunter2", "COUNT=-1", "RATIO=0.125", "TIMEOUT=1500us",
		"RETRY=00:00:00.5", "SHARDS=[\"a\"]", "TAGS=a;b;c", "VERBOSE=yes")
	want := []string{
		"docuconf: 10 configuration problems:",
		"  NAME: does not fit in T-NAME (PIC X(5)) (out_of_range)",
		"  TOKEN: does not fit in T-TOKEN (PIC X(3)) (out_of_range)",
		"  COUNT: is negative, and T-COUNT is unsigned (out_of_range)",
		"  RATIO: has more decimal places than T-RATIO holds (out_of_range)",
		"  TIMEOUT: is finer than the ms T-TIMEOUT counts in (out_of_range)",
		"  RETRY: is finer than the s T-RETRY counts in (out_of_range)",
		"  VERBOSE: is not true or false (invalid_type)",
		"  SHARDS: has an item of the wrong JSON type (invalid_type)",
		"  TAGS: has more items than its COBOL table holds (too_many_items)",
		"  REGION: is required but not set (missing_required)",
	}
	if got := strings.TrimRight(errOut, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("stderr:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}
	if !strings.HasPrefix(out, "RC=+000000001") {
		t.Errorf("RETURN-CODE: %s", out)
	}
	if strings.Contains(errOut, "hunter2") {
		t.Error("a secret value was printed")
	}
}

// boundsProgram compiles testdata/BNDCFGC.cpy, a mainframe-style
// copybook (sequence numbers, columns 73-80, no level-01 record, VALUE
// defaults, tags in an inline comment), with a program that COPYs it
// under its own 01, as such copybooks are used.
func boundsProgram(t *testing.T) string {
	t.Helper()
	cobc, err := exec.LookPath("cobc")
	if err != nil {
		skipOrFail(t, "cobc (GnuCOBOL) not found")
	}
	dir := t.TempDir()
	src, err := os.ReadFile("testdata/BNDCFGC.cpy")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "BNDCFGC.cpy"), src, 0o644)
	c, err := gen.Load(filepath.Join(dir, "BNDCFGC.cpy"), gen.Options{})
	if err != nil {
		t.Fatal(err)
	}
	loader, err := c.Loader()
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "BNDCFG.cbl"), loader, 0o644)
	os.WriteFile(filepath.Join(dir, "main.cbl"), []byte(`       IDENTIFICATION DIVISION.
       PROGRAM-ID. MAIN.
       DATA DIVISION.
       WORKING-STORAGE SECTION.
       01  WS-CFG.
       COPY BNDCFGC.
       01  K PIC 9.
       PROCEDURE DIVISION.
           CALL "BNDCFG" USING WS-CFG
           DISPLAY "RC=" RETURN-CODE END-DISPLAY
           DISPLAY "WORKERS=" BC-WORKER-COUNT END-DISPLAY
           DISPLAY "RATIO=" BC-RATIO END-DISPLAY
           DISPLAY "TIMEOUT=" BC-TIMEOUT END-DISPLAY
           DISPLAY "PORT=" BC-PORT END-DISPLAY
           DISPLAY "VERBOSE=" BC-VERBOSE END-DISPLAY
           DISPLAY "SHARDS=" BC-SHARD-COUNT END-DISPLAY
           MOVE 0 TO RETURN-CODE
           STOP RUN.
`), 0o644)
	cmd := exec.Command(cobc, "-x", "-o", "main", "main.cbl", "BNDCFG.cbl")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cobc: %v\n%s", err, out)
	}
	return filepath.Join(dir, "main")
}

// TestLoaderDefaultsFromValue: VALUE clauses and an inline @default are
// the defaults.
func TestLoaderDefaultsFromValue(t *testing.T) {
	bin := boundsProgram(t)
	out, errOut := runLoader(t, bin)
	want := "RC=+000000000\nWORKERS=+0004\nRATIO=0.50\nTIMEOUT=0030000\nPORT=00080\nVERBOSE=Y\nSHARDS=0\n"
	if out != want || errOut != "" {
		t.Fatalf("stdout:\n%s\nstderr:\n%s\nwant:\n%s", out, errOut, want)
	}
}

// TestLoaderBounds: without docuconf exec, the loader still enforces
// @min, @max, @item-min and @item-max, and writes the termination log.
func TestLoaderBounds(t *testing.T) {
	bin := boundsProgram(t)
	tlog := filepath.Join(t.TempDir(), "termination-log")
	out, errOut := runLoader(t, bin, "WORKER_COUNT=500", "RATIO=0.95", "TIMEOUT=10m", "SHARDS=5,12",
		"DOCUCONF_TERMINATION_LOG="+tlog)
	want := `docuconf: 4 configuration problems:
  WORKER_COUNT: is above max 64 (out_of_range)
  RATIO: is above max 0.9 (out_of_range)
  TIMEOUT: is above max 5m (out_of_range)
  SHARDS: has an item above itemMax 9 (out_of_range)
`
	if errOut != want || !strings.HasPrefix(out, "RC=+000000001") {
		t.Fatalf("stdout:\n%s\nstderr:\n%s\nwant:\n%s", out, errOut, want)
	}
	if got, _ := os.ReadFile(tlog); string(got) != want {
		t.Errorf("termination log:\n%s\nwant:\n%s", got, want)
	}
	_, errOut = runLoader(t, bin, "WORKER_COUNT=0", "RATIO=0.05", "TIMEOUT=500ms", "SHARDS=-1")
	for _, w := range []string{"WORKER_COUNT: is below min 1", "RATIO: is below min 0.1", "TIMEOUT: is below min 1s", "SHARDS: has an item below itemMin 0"} {
		if !strings.Contains(errOut, "  "+w+" (out_of_range)\n") {
			t.Errorf("want %q in:\n%s", w, errOut)
		}
	}
}

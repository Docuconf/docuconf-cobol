package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAndCheck(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile("../../examples/orders/orders-config.cpy")
	if err != nil {
		t.Fatal(err)
	}
	cpy := filepath.Join(dir, "orders-config.cpy")
	os.WriteFile(cpy, src, 0o644)

	var out, errOut bytes.Buffer
	if code := run([]string{"generate", "-check", cpy}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "out of date") {
		t.Fatalf("check before generate: exit %d\n%s", code, errOut.String())
	}
	out.Reset()
	if code := run([]string{"generate", cpy}, &out, &errOut); code != 0 {
		t.Fatalf("generate: exit %d\n%s", code, errOut.String())
	}
	for _, f := range []string{"contract.cue", "ORDCFG.cbl"} {
		if !strings.Contains(out.String(), "wrote "+filepath.Join(dir, f)) {
			t.Errorf("did not write %s:\n%s", f, out.String())
		}
	}
	errOut.Reset()
	if code := run([]string{"generate", "-check", cpy}, &out, &errOut); code != 0 {
		t.Fatalf("check after generate: exit %d\n%s", code, errOut.String())
	}
}

func TestGenerateReportsProblems(t *testing.T) {
	cpy := filepath.Join(t.TempDir(), "bad.cpy")
	os.WriteFile(cpy, []byte("       01  BAD-CONFIG.\n           05  CFG-PORT PIC 9(5).\n"), 0o644)
	var out, errOut bytes.Buffer
	code := run([]string{"generate", cpy}, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "bad.cpy:2: CFG-PORT: needs a description") ||
		!strings.Contains(errOut.String(), "name the service") {
		t.Fatalf("exit %d\n%s", code, errOut.String())
	}
}

// Flags may follow the copybook; -o creates its directory; -check names
// the exact command that regenerates.
func TestGenerateFlagsAnywhere(t *testing.T) {
	dir := t.TempDir()
	src, _ := os.ReadFile("../../examples/orders/orders-config.cpy")
	cpy := filepath.Join(dir, "orders-config.cpy")
	os.WriteFile(cpy, src, 0o644)
	out := filepath.Join(dir, "gen", "out")
	var o, e bytes.Buffer
	if code := run([]string{"generate", cpy, "-o", out, "-contract", "orders.cue"}, &o, &e); code != 0 {
		t.Fatalf("exit %d\n%s", code, e.String())
	}
	if _, err := os.Stat(filepath.Join(out, "orders.cue")); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(out, "ORDCFG.cbl"))
	e.Reset()
	if code := run([]string{"generate", "-check", cpy, "-o", out, "-contract", "orders.cue"}, &o, &e); code != 1 ||
		!strings.Contains(e.String(), "out of date; run: docuconf-cobol generate "+cpy+" -o "+out+" -contract orders.cue\n") {
		t.Fatalf("exit %d\n%s", code, e.String())
	}
}

func TestRuntimeCopybooks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "copylib")
	var o, e bytes.Buffer
	if code := run([]string{"runtime", "-o", dir}, &o, &e); code != 0 {
		t.Fatalf("exit %d\n%s", code, e.String())
	}
	for _, f := range []string{"DCRTWS.cpy", "DCRTPD.cpy"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Error(err)
		}
	}
}

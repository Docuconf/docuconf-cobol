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

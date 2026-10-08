package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docuconf/docuconf-cobol/internal/gen"
)

// TestDetailsContractFirst: a contract with details passes docuconf
// check, the contract-first loader docuconf exec uses, and the details
// change nothing at runtime; docuconf docs shows them.
func TestDetailsContractFirst(t *testing.T) {
	_, dc := tools(t)
	src := "      *> @service demo  @prefix CFG-\n       01  DEMO-CONFIG.\n" +
		"      *> Number of workers\n      *>\n      *> Keep it at or below the **pool size**.\n" +
		"      *> @min 1  @max 64  @default 4\n           05  CFG-WORKERS PIC 9(2).\n"
	c, err := gen.Build("demo.cpy", src, gen.Options{})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := c.ContractCUE()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cue), `details:     "Keep it at or below the **pool size**."`) {
		t.Fatalf("no details in:\n%s", cue)
	}
	contract := filepath.Join(t.TempDir(), "contract.cue")
	if err := os.WriteFile(contract, cue, 0o644); err != nil {
		t.Fatal(err)
	}
	check := func(env ...string) (string, error) {
		cmd := exec.Command(dc, "check", "-contract", contract)
		cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "DOCUCONF_TERMINATION_LOG=-"}, env...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := check("WORKERS=8"); err != nil {
		t.Errorf("WORKERS=8: %v\n%s", err, out)
	}
	if out, err := check("WORKERS=65"); err == nil || !strings.Contains(out, "WORKERS: 65 is above max 64 (out_of_range)") {
		t.Errorf("WORKERS=65: %v\n%s", err, out)
	}
	out, err := exec.Command(dc, "docs", contract, "--format", "markdown").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Keep it at or below the **pool size**.") {
		t.Errorf("docuconf docs: %v\n%s", err, out)
	}
}

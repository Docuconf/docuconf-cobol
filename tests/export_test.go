package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/docuconf/docuconf-cobol/internal/gen"
)

// knownExportDifferences are where a COBOL declaration cannot match the
// shared export fixture, which assumes unbounded host types and a host
// that reloads files:
//
//   - a COBOL field always has a size, which the contract states (SPEC
//     section 5: an SDK MUST export the range its field holds): OLD_PORT
//     is a PIC 9(5), PARTNER_PASSWORD a PIC X(128) and SHARDS an
//     OCCURS 64 table;
//   - the loader reads file paths once and nothing rereads a file's
//     checks, so generate rejects reload: watch (SPEC section 11.2 item
//     8), and settings and serving-tls export restart.
//
// Every other field of the fixture must match the golden contract.
var knownExportDifferences = []string{
	`files.serving-tls.reload: golden "watch", exported "restart"`,
	`files.settings.reload: golden "watch", exported "restart"`,
	`vars.OLD_PORT.max: not in the golden contract (exported 99999)`,
	`vars.OLD_PORT.min: not in the golden contract (exported 0)`,
	`vars.PARTNER_PASSWORD.maxLength: not in the golden contract (exported 128)`,
	`vars.SHARDS.maxItems: not in the golden contract (exported 64)`,
}

// TestExportFixture declares the shared export fixture
// (conformance/export/fixture.yaml) as testdata/export/FIXTURE.cpy,
// generates its contract and compares it with the golden contract using
// docuconf conformance export (SPEC section 11.2 item 3). The golden
// contract is DOCUCONF_EXPORT_GOLDEN, else export/golden.cue next to
// DOCUCONF_CONFORMANCE's cases.json.
func TestExportFixture(t *testing.T) {
	_, dc := tools(t)
	golden := os.Getenv("DOCUCONF_EXPORT_GOLDEN")
	if golden == "" {
		cases := os.Getenv("DOCUCONF_CONFORMANCE")
		if cases == "" {
			cases = filepath.Join("..", "..", "docuconf-go", "conformance", "cases.json")
		}
		golden = filepath.Join(filepath.Dir(cases), "export", "golden.cue")
	}
	if _, err := os.Stat(golden); err != nil {
		skipOrFail(t, "the export golden contract was not found: %v", err)
	}
	c, err := gen.Load("testdata/export/FIXTURE.cpy", gen.Options{})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := c.ContractCUE()
	if err != nil {
		t.Fatal(err)
	}
	exported := filepath.Join(t.TempDir(), "contract.cue")
	if err := os.WriteFile(exported, cue, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(dc, "conformance", "export", "--golden", golden, exported)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	var diffs []string
	for _, l := range strings.Split(out.String(), "\n") {
		if l != "" && !strings.HasPrefix(l, "docuconf conformance:") {
			diffs = append(diffs, l)
		}
	}
	slices.Sort(diffs)
	if !slices.Equal(diffs, knownExportDifferences) {
		t.Errorf("docuconf conformance export (%v):\n%s\nwant exactly the known differences:\n%s",
			err, out.String(), strings.Join(knownExportDifferences, "\n"))
	}
}

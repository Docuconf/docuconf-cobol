package tests

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestShopExample runs examples/shop's matrix: SHOP under docuconf exec
// with contract.json, each case with an explicit environment and a fresh
// empty file root. docuconf exec enforces the roles, the requiredIf
// conditions and the otel@1 fragment; SHOP prints its line.
func TestShopExample(t *testing.T) {
	cobc, dc := tools(t)
	example, err := filepath.Abs("../examples/shop")
	if err != nil {
		t.Fatal(err)
	}
	shop := filepath.Join(t.TempDir(), "shop")
	if out, err := exec.Command(cobc, "-x", "-o", shop, filepath.Join(example, "SHOP.cbl")).CombinedOutput(); err != nil {
		t.Fatalf("cobc: %v\n%s", err, out)
	}

	const dbURL = "postgres://shop@db:5432/shop"
	// problem is the one line docuconf exec reports; the header says it
	// is the only one.
	cases := []struct {
		name    string
		env     []string
		keyFile bool   // write a 40-character signing key under the root
		stdout  string // the line SHOP prints, when the config is valid
		problem string // the single problem docuconf exec reports, otherwise
	}{
		{name: "1 web", env: []string{"DOCUCONF_ROLE=web", "ALLOWED_ORIGINS=https://shop.example"},
			stdout: "shop web: port 8080, origins https://shop.example\n"},
		{name: "2 worker needs no ALLOWED_ORIGINS", env: []string{"DOCUCONF_ROLE=worker", "QUEUES=default,mail"},
			stdout: "shop worker: queues default,mail, webhooks off\n"},
		{name: "3 web without ALLOWED_ORIGINS", env: []string{"DOCUCONF_ROLE=web"},
			problem: "ALLOWED_ORIGINS: is required but not set (missing_required)"},
		{name: "4 no role", env: []string{"QUEUES=default"},
			problem: "DOCUCONF_ROLE: is required but not set (missing_required)"},
		{name: "5 unknown role", env: []string{"DOCUCONF_ROLE=cron"},
			problem: `DOCUCONF_ROLE: "cron" is not one of web, worker (not_in_enum)`},
		{name: "6 json logs without a schema", env: []string{"DOCUCONF_ROLE=worker", "QUEUES=default", "LOG_FORMAT=json"},
			problem: `LOG_SCHEMA_URL: is required when LOG_FORMAT is "json" but not set (missing_required)`},
		{name: "7 json logs with a schema", env: []string{"DOCUCONF_ROLE=worker", "QUEUES=default", "LOG_FORMAT=json", "LOG_SCHEMA_URL=https://logs.example/v1.json"},
			stdout: "shop worker: queues default, webhooks off\n"},
		{name: "8 webhooks without a key", env: []string{"DOCUCONF_ROLE=worker", "QUEUES=default", "WEBHOOKS_ENABLED=true"},
			problem: "webhook-signing-key: {root}/etc/shop/webhooks/signing.key does not exist, and is required when WEBHOOKS_ENABLED is true (file_missing)"},
		{name: "9 webhooks with a key", env: []string{"DOCUCONF_ROLE=worker", "QUEUES=default", "WEBHOOKS_ENABLED=true"}, keyFile: true,
			stdout: "shop worker: queues default, webhooks on\n"},
		{name: "10 the key belongs to the worker", env: []string{"DOCUCONF_ROLE=web", "ALLOWED_ORIGINS=https://shop.example", "WEBHOOKS_ENABLED=true"},
			stdout: "shop web: port 8080, origins https://shop.example\n"},
		{name: "11 fragment vars are checked", env: []string{"DOCUCONF_ROLE=web", "ALLOWED_ORIGINS=https://shop.example", "OTEL_EXPORTER_OTLP_PROTOCOL=carrier-pigeon"},
			problem: `OTEL_EXPORTER_OTLP_PROTOCOL: "carrier-pigeon" is not one of grpc, http/protobuf, http/json (not_in_enum)`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			if c.keyFile {
				dir := filepath.Join(root, "etc", "shop", "webhooks")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "signing.key"), []byte(strings.Repeat("k", 40)), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command(dc, "exec", "-contract", "contract.json", "--", shop)
			cmd.Dir = example
			cmd.Env = append([]string{
				"PATH=" + os.Getenv("PATH"),
				"DATABASE_URL=" + dbURL,
				"DOCUCONF_FILE_ROOT=" + root,
				"DOCUCONF_TERMINATION_LOG=-",
			}, c.env...)
			var stdout, stderr strings.Builder
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			code := 0
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				code = exit.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(stdout.String()+stderr.String(), dbURL) {
				t.Errorf("the secret DATABASE_URL was printed")
			}
			if c.problem == "" {
				if code != 0 || stdout.String() != c.stdout {
					t.Errorf("exit %d, stdout %q, stderr %q; want exit 0 and %q", code, stdout.String(), stderr.String(), c.stdout)
				}
				return
			}
			want := "docuconf: 1 configuration problem:\n  " + strings.ReplaceAll(c.problem, "{root}", root) + "\n"
			if code != 1 || stderr.String() != want || stdout.String() != "" {
				t.Errorf("exit %d, stdout %q, stderr:\n%s\nwant exit 1 and:\n%s", code, stdout.String(), stderr.String(), want)
			}
		})
	}
}

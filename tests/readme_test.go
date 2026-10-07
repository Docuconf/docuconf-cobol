package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A block is one fenced code block of a README.
type block struct {
	lang, body string
	marker     string // the <!-- ... --> comment on the line above, if any
	line       int
}

var fenceRe = regexp.MustCompile("(?m)^```([a-z]*)\n")

func readBlocks(t *testing.T, path string) []block {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	var out []block
	for {
		loc := fenceRe.FindStringSubmatchIndex(s)
		if loc == nil {
			return out
		}
		lang := s[loc[2]:loc[3]]
		rest := s[loc[1]:]
		end := strings.Index(rest, "\n```\n")
		if end < 0 {
			t.Fatalf("%s: unclosed code block", path)
		}
		before := strings.TrimRight(s[:loc[0]], "\n")
		marker := ""
		if i := strings.LastIndexByte(before, '\n'); strings.HasPrefix(before[i+1:], "<!--") {
			marker = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(before[i+1:], "<!--"), "-->"))
		}
		line := strings.Count(string(data[:len(data)-len(s)+loc[0]]), "\n") + 1
		out = append(out, block{lang: lang, body: rest[:end+1], marker: marker, line: line})
		s = rest[end+5:]
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestREADMESnippets checks every code block of the README and of the
// example's walkthrough: COBOL and Dockerfile blocks are pieces of the
// example's files, which CI compiles and runs; shell blocks are run in a
// copy of the example and their output compared with the text block
// after them, or are pieces of scripts CI runs; install commands pin
// the docuconf-go commit go.mod pins.
func TestREADMESnippets(t *testing.T) {
	_, dc := tools(t)
	bin := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "docuconf-cobol"), "../cmd/docuconf-cobol")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	if err := os.Symlink(dc, filepath.Join(bin, "docuconf")); err != nil {
		t.Fatal(err)
	}
	pathEnv := bin + string(os.PathListSeparator) + os.Getenv("PATH")

	example := "../examples/orders"
	sources := map[string][]string{
		"cobol":      {example + "/orders-config.cpy", example + "/ORDERS-BATCH.cbl", example + "/CFGTEST.cbl"},
		"dockerfile": {example + "/Dockerfile"},
	}
	scripts := readFile(t, example+"/smoke.sh") + readFile(t, example+"/test-config.sh")
	ci := readFile(t, "../.github/workflows/ci.yml")
	pin := regexp.MustCompile(`github.com/docuconf/docuconf-go (v\S+)`).FindStringSubmatch(readFile(t, "../go.mod"))[1]

	for _, readme := range []string{"../README.md", example + "/README.md"} {
		blocks := readBlocks(t, readme)
		// Shell blocks run in sequence in one copy of the example.
		work := t.TempDir()
		if out, err := exec.Command("cp", "-r", example+"/.", work).CombinedOutput(); err != nil {
			t.Fatalf("cp: %v\n%s", err, out)
		}
		for i, b := range blocks {
			where := readme + ":" + strconv.Itoa(b.line)
			switch b.lang {
			case "cobol", "dockerfile":
				found := false
				for _, f := range sources[b.lang] {
					if strings.Contains(readFile(t, f), b.body) {
						found = true
					}
				}
				if !found {
					t.Errorf("%s: this %s block is not a piece of %v:\n%s", where, b.lang, sources[b.lang], b.body)
				}
			case "usage":
				var out bytes.Buffer
				cmd := exec.Command(filepath.Join(bin, "docuconf-cobol"), "generate", "-h")
				cmd.Stderr = &out
				cmd.Run()
				for _, f := range regexp.MustCompile(`\[(-[a-z]+)`).FindAllStringSubmatch(b.body, -1) {
					if !strings.Contains(out.String(), "  "+f[1]+" ") && !strings.Contains(out.String(), "  "+f[1]+"\n") {
						t.Errorf("%s: generate has no flag %s", where, f[1])
					}
				}
			case "text":
				if strings.HasPrefix(b.marker, "generate: ") {
					cmd := exec.Command(filepath.Join(bin, "docuconf-cobol"), "generate", "-o", t.TempDir(), filepath.Join("..", strings.TrimPrefix(b.marker, "generate: ")))
					out, _ := cmd.CombinedOutput()
					if string(out) != b.body {
						t.Errorf("%s: generate prints:\n%s\nthe README shows:\n%s", where, out, b.body)
					}
				} else if i == 0 || blocks[i-1].lang != "sh" {
					t.Errorf("%s: a text block must follow a shell block, or name its command", where)
				}
			case "sh":
				lines := commandLines(b.body)
				switch {
				case strings.HasPrefix(b.marker, "not executed"):
					for _, l := range lines {
						if c := stripAssignments(l); c != "" && !strings.Contains(ci, c) {
							t.Errorf("%s: %q is not run by CI", where, c)
						}
					}
				case allPrefixed(lines, "go install "):
					for _, l := range lines {
						if strings.Contains(l, "docuconf-go/cmd/docuconf@") && !strings.HasSuffix(l, "@"+pin) {
							t.Errorf("%s: %q does not install the pinned %s", where, l, pin)
						}
					}
				case allContained(b.body, scripts):
				default:
					cmd := exec.Command("sh", "-c", b.body)
					cmd.Dir = work
					cmd.Env = []string{"PATH=" + pathEnv, "HOME=" + os.Getenv("HOME"), "DOCUCONF_TERMINATION_LOG=-"}
					out, err := cmd.CombinedOutput()
					if i+1 < len(blocks) && blocks[i+1].lang == "text" && blocks[i+1].marker == "" {
						if got := stripCompilerNoise(string(out)); got != blocks[i+1].body {
							t.Errorf("%s: the command prints:\n%s\nthe README shows:\n%s", where, got, blocks[i+1].body)
						}
					} else if err != nil {
						t.Errorf("%s: %v\n%s", where, err, out)
					}
				}
			case "yaml":
				var v any
				if err := yaml.Unmarshal([]byte(b.body), &v); err != nil {
					t.Errorf("%s: %v", where, err)
				}
			default:
				t.Errorf("%s: no check for a %q block; mark it cobol, dockerfile, sh, text, usage or yaml", where, b.lang)
			}
		}
	}
}

// TestCronJobMatchesRender: the example CronJob carries what docuconf
// render writes (test-config.sh checks render against rendered.yaml).
func TestCronJobMatchesRender(t *testing.T) {
	var rendered struct {
		Env          any `yaml:"env"`
		Volumes      any `yaml:"volumes"`
		VolumeMounts any `yaml:"volumeMounts"`
	}
	if err := yaml.Unmarshal([]byte(readFile(t, "../examples/orders/k8s/rendered.yaml")), &rendered); err != nil {
		t.Fatal(err)
	}
	var cj struct {
		Spec struct {
			JobTemplate struct {
				Spec struct {
					Template struct {
						Spec struct {
							Containers []struct {
								Env          any `yaml:"env"`
								VolumeMounts any `yaml:"volumeMounts"`
							}
							Volumes any `yaml:"volumes"`
						}
					}
				} `yaml:"spec"`
			} `yaml:"jobTemplate"`
		}
	}
	if err := yaml.Unmarshal([]byte(readFile(t, "../examples/orders/k8s/cronjob.yaml")), &cj); err != nil {
		t.Fatal(err)
	}
	pod := cj.Spec.JobTemplate.Spec.Template.Spec
	a, _ := yaml.Marshal([]any{rendered.Env, rendered.VolumeMounts, rendered.Volumes})
	b, _ := yaml.Marshal([]any{pod.Containers[0].Env, pod.Containers[0].VolumeMounts, pod.Volumes})
	if string(a) != string(b) {
		t.Errorf("k8s/cronjob.yaml differs from k8s/rendered.yaml:\n%s\nwant:\n%s", b, a)
	}
}

// commandLines joins continuation lines and drops comments and blanks.
func commandLines(s string) []string {
	var out []string
	cur := ""
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if cur == "" && (l == "" || strings.HasPrefix(l, "#")) {
			continue
		}
		if strings.HasSuffix(l, "\\") {
			cur += strings.TrimSuffix(l, "\\")
			continue
		}
		out = append(out, strings.TrimSpace(cur+l))
		cur = ""
	}
	return out
}

var assignRe = regexp.MustCompile(`^([A-Z_]+=(\$\([^)]*\)|\S*)\s*)+`)

func stripAssignments(l string) string { return strings.TrimSpace(assignRe.ReplaceAllString(l, "")) }

func allPrefixed(lines []string, p string) bool {
	for _, l := range lines {
		if !strings.HasPrefix(l, p) {
			return false
		}
	}
	return len(lines) > 0
}

func allContained(body, in string) bool {
	for _, l := range strings.Split(strings.TrimSpace(body), "\n") {
		if !strings.Contains(in, strings.TrimSpace(l)) {
			return false
		}
	}
	return true
}

// stripCompilerNoise drops the warnings some distributions' cobc prints
// about its own C flags.
func stripCompilerNoise(s string) string {
	var out []string
	for _, l := range strings.SplitAfter(s, "\n") {
		if strings.Contains(l, "_FORTIFY_SOURCE") || strings.Contains(l, "this is the location of the previous definition") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "")
}

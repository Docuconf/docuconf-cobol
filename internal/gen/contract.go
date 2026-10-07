package gen

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	docuconf "github.com/docuconf/docuconf-go"
)

// Version is the docuconf-cobol version recorded in contracts.
const Version = "0.1.0"

// ContractJSON returns the contract document.
func (c *Config) ContractJSON() ([]byte, error) {
	vars := map[string]any{}
	for _, v := range c.Vars {
		vars[v.Env] = v.contract
	}
	doc := map[string]any{
		"apiVersion": "docuconf.dev/v1alpha1",
		"kind":       "ConfigContract",
		"metadata": map[string]any{
			"name":      c.Service,
			"generator": map[string]any{"language": "cobol", "sdk": "docuconf-cobol", "version": Version},
		},
		"vars": vars,
	}
	if len(c.Files) > 0 {
		files := map[string]any{}
		for _, f := range c.Files {
			files[f.Name] = f.contract
		}
		doc["files"] = files
	}
	return json.MarshalIndent(doc, "", "  ")
}

var problemRe = regexp.MustCompile(`^(file input )?([A-Za-z0-9_-]+): `)

// ContractCUE returns contract.cue, written by the Go SDK's ContractCUE,
// which checks the contract as the SDKs' contract-first mode does. Its
// problems are reported against the copybook line that declares them.
func (c *Config) ContractCUE() ([]byte, error) {
	doc, err := c.ContractJSON()
	if err != nil {
		return nil, err
	}
	out, err := docuconf.ContractCUE(doc, c.Package)
	if de, ok := err.(*docuconf.DeclarationError); ok {
		lines := map[string]string{}
		for _, v := range c.Vars {
			lines[v.Env] = fmt.Sprintf("%s:%d: %s (%s)", c.Copybook, v.Line, v.Field, v.Env)
		}
		for _, f := range c.Files {
			lines[f.Name] = fmt.Sprintf("%s:%d: %s (file input %s)", c.Copybook, f.Line, f.Field, f.Name)
		}
		var ps []string
		for _, p := range de.Problems {
			if m := problemRe.FindStringSubmatch(p); m != nil && lines[m[2]] != "" {
				p = lines[m[2]] + ": " + strings.TrimPrefix(p, m[0])
			}
			ps = append(ps, p)
		}
		return nil, &Error{ps}
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

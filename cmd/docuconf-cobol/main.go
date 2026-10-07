// Command docuconf-cobol generates a docuconf contract and a loader
// program from an annotated COBOL copybook.
//
//	docuconf-cobol generate [flags] orders-config.cpy
//
// writes contract.cue, for the platform and for docuconf exec, and
// <PROGRAM>.cbl, which the app CALLs to read its configuration.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docuconf/docuconf-cobol/copybooks"
	"github.com/docuconf/docuconf-cobol/internal/copybook"
	"github.com/docuconf/docuconf-cobol/internal/gen"
)

const usage = `docuconf-cobol: docuconf contracts and loaders for COBOL.

Usage:
  docuconf-cobol generate [flags] <copybook>
  docuconf-cobol runtime [-o dir]
  docuconf-cobol version

generate reads the annotated configuration record in <copybook> and
writes contract.cue and <PROGRAM>.cbl next to it (or in -o). With
-check it writes nothing, and fails if either file is out of date.
Run "docuconf-cobol generate -h" for its flags.

runtime writes the loader runtime copybooks DCRTWS.cpy and DCRTPD.cpy,
for loaders generated with -runtime copy.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

var errStale = errors.New("generated files are out of date")

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "generate":
		err := generate(args[1:], stdout, stderr)
		switch {
		case err == nil:
			return 0
		case errors.Is(err, flag.ErrHelp):
			return 0
		case errors.Is(err, errStale):
			return 1
		}
		var ge *gen.Error
		var ce *copybook.Error
		if errors.As(err, &ge) || errors.As(err, &ce) {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stderr, "docuconf-cobol generate: %v\n", err)
		return 2
	case "runtime":
		if err := writeRuntime(args[1:], stdout, stderr); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			fmt.Fprintf(stderr, "docuconf-cobol runtime: %v\n", err)
			return 2
		}
		return 0
	case "version":
		fmt.Fprintln(stdout, "docuconf-cobol", gen.Version)
		return 0
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "docuconf-cobol: unknown command %q\n\n%s", args[0], usage)
	return 2
}

func generate(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		free     = fs.Bool("free", false, "the copybook is in free format (cobc -free); the default is fixed")
		name     = fs.String("name", "", "service name, a DNS label (default: @service on the record)")
		program  = fs.String("program", "", "PROGRAM-ID of the loader (default: @program, else <MEMBER>L for a copybook named like a PDS member, else LOAD-<record>)")
		pkg      = fs.String("package", "", "CUE package of contract.cue (default: the service name)")
		prefix   = fs.String("prefix", "", "data-name prefix to drop when deriving variable names (default: @prefix)")
		record   = fs.String("record", "", "the level-01 record to read, when the copybook has several; for a copybook of fields with no 01, the name the loader gives the record (default DC-RECORD)")
		runtime  = fs.String("runtime", "inline", "inline: the loader holds the docuconf runtime; copy: it COPYs DCRTWS and DCRTPD (see docuconf-cobol runtime)")
		out      = fs.String("o", "", "directory to write into (default: the copybook's directory)")
		contract = fs.String("contract", "contract.cue", "contract file name, in the output directory")
		check    = fs.Bool("check", false, "write nothing; fail if the files on disk are out of date")
	)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: docuconf-cobol generate [flags] <copybook>")
		fs.PrintDefaults()
	}
	// Flags may come before or after the copybook.
	var pos []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(pos) != 1 {
		fs.Usage()
		return fmt.Errorf("give exactly one copybook, not %d", len(pos))
	}
	src := pos[0]
	opts := gen.Options{Service: *name, Program: *program, Package: *pkg, Prefix: *prefix, Record: *record, Runtime: *runtime}
	if *free {
		opts.Format = copybook.Free
	}
	c, err := gen.Load(src, opts)
	if err != nil {
		return err
	}
	for _, w := range c.Warnings {
		fmt.Fprintln(stderr, w)
	}
	cue, err := c.ContractCUE()
	if err != nil {
		return err
	}
	loader, err := c.Loader()
	if err != nil {
		return err
	}
	dir := *out
	if dir == "" {
		dir = filepath.Dir(src)
	}
	files := []struct {
		path string
		data []byte
	}{
		{filepath.Join(dir, *contract), cue},
		{filepath.Join(dir, c.Program+".cbl"), loader},
	}
	stale := false
	for _, f := range files {
		if *check {
			old, err := os.ReadFile(f.path)
			if err != nil || !bytes.Equal(old, f.data) {
				fmt.Fprintf(stderr, "%s is out of date; run: %s\n", f.path, regenerate(args))
				stale = true
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(f.path, f.data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "wrote %s\n", f.path)
	}
	if stale {
		return errStale
	}
	if *check {
		fmt.Fprintf(stdout, "%s: generated files are up to date\n", src)
	}
	return nil
}

// regenerate is the command that rewrites the files -check found out of
// date: the same arguments, without -check.
func regenerate(args []string) string {
	cmd := []string{"docuconf-cobol", "generate"}
	for _, a := range args {
		if a == "-check" || a == "--check" || a == "-check=true" || a == "--check=true" {
			continue
		}
		if strings.ContainsAny(a, " '\"$") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		cmd = append(cmd, a)
	}
	return strings.Join(cmd, " ")
}

// writeRuntime writes the runtime copybooks that a loader generated with
// -runtime copy COPYs.
func writeRuntime(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("runtime", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", ".", "directory to write DCRTWS.cpy and DCRTPD.cpy into")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments %q", fs.Args())
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	for name, data := range map[string]string{"DCRTWS.cpy": copybooks.WorkingStorage, "DCRTPD.cpy": copybooks.Procedures} {
		p := filepath.Join(*out, name)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "wrote %s\n", p)
	}
	return nil
}

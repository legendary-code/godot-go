// godot-go is the user-facing codegen tool: when invoked via
// `//go:generate godot-go`, it walks the package directory, discovers
// every `@class` struct across all *.go files, and emits a single
// `bindings.gen.go` at the package root with the GDExtension
// class-registration glue for all of them.
//
// One `//go:generate godot-go` per package is enough — convention is to
// place it in a dedicated `generate.go` (or `doc.go`) since `go generate`
// walks every file in the package and only one trigger is needed.
//
// Flags:
//
//	-dir <path>   override the package directory (defaults to the directory of $GOFILE, or $PWD)
//	-out <path>   override the output path (defaults to <dir>/bindings.gen.go)
//	-print        print the discovery report to stdout instead of writing
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go/parser"
	"go/token"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "godot-go:", err)
		os.Exit(1)
	}
}

func run() error {
	dirFlag := flag.String("dir", "", "Override the package directory (defaults to the directory of $GOFILE, or $PWD)")
	outFlag := flag.String("out", "", "Override the output path (defaults to <dir>/bindings.gen.go)")
	printFlag := flag.Bool("print", false, "Print the discovery report to stdout instead of writing the binding file")
	flag.Parse()

	pkgDir := *dirFlag
	if pkgDir == "" {
		if gofile := os.Getenv("GOFILE"); gofile != "" {
			abs, err := filepath.Abs(gofile)
			if err != nil {
				return fmt.Errorf("resolve $GOFILE %q: %w", gofile, err)
			}
			pkgDir = filepath.Dir(abs)
		}
	}
	if pkgDir == "" {
		var err error
		pkgDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve cwd: %w", err)
		}
	}
	pkgDir, err := filepath.Abs(pkgDir)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", pkgDir, err)
	}

	outPath := *outFlag
	if outPath == "" {
		outPath = filepath.Join(pkgDir, "bindings.gen.go")
	}

	// Walk the package directory and discover every @class file. The
	// generated output is excluded so a re-run doesn't re-discover its
	// own previously-emitted classes; _test.go files are also skipped.
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", pkgDir, err)
	}
	var sources []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		full := filepath.Join(pkgDir, name)
		if full == outPath {
			continue
		}
		sources = append(sources, full)
	}
	sort.Strings(sources) // stable iteration order so the emitted file is deterministic

	fset := token.NewFileSet()
	pkgName := os.Getenv("GOPACKAGE")

	var classes []*discovered
	for _, src := range sources {
		file, err := parser.ParseFile(fset, src, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", src, err)
		}
		if pkgName == "" {
			pkgName = file.Name.Name
		} else if file.Name.Name != pkgName {
			// Mixed packages in one directory — not supported by go
			// build either, so just surface a clear error rather than
			// pick one arbitrarily.
			return fmt.Errorf("%s declares package %q but %q expected from sibling files", src, file.Name.Name, pkgName)
		}
		// discover() returns nil-MainClass for files with no @class;
		// those are valid (helper files) but skipped by emit. The
		// existing single-class error message is wrong for package
		// mode, so we tolerate "no @class struct found" here and
		// validate below that *some* file produced one.
		d, err := discover(fset, file, pkgName)
		if err != nil {
			if strings.Contains(err.Error(), "no @class struct found") {
				continue
			}
			return err
		}
		if d == nil || d.MainClass == nil {
			continue
		}
		classes = append(classes, d)
	}

	if len(classes) == 0 {
		return fmt.Errorf("no @class structs found in package %q (looked at %d source files)", pkgDir, len(sources))
	}

	if *printFlag {
		for _, d := range classes {
			report(os.Stdout, fset, d)
		}
		return nil
	}

	out, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer out.Close()
	if err := emit(out, fset, classes); err != nil {
		return fmt.Errorf("emit %s: %w", outPath, err)
	}
	return nil
}

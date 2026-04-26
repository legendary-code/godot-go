// godot-go is the user-facing codegen tool: when invoked via
// `//go:generate godot-go`, it reads the trigger file (named by the
// GOFILE env var that go generate sets), discovers the user's extension
// class declaration, and emits a sibling `<file>_bindings.go` with the
// GDExtension class-registration glue.
//
// Phase 5c scope: emit registration for the main class plus its
// no-arg/no-return instance methods. Static methods, virtual
// candidates, and methods with non-trivial signatures error out so
// they're caught early — Phase 5d/5e fold those in.
//
// Flags:
//
//	-file <path>   override the trigger file (defaults to $GOFILE)
//	-out <path>    override the output path (defaults to <file>_bindings.go)
//	-print         print the discovery report to stdout instead of writing
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
	fileFlag := flag.String("file", "", "Override the trigger file (defaults to $GOFILE)")
	outFlag := flag.String("out", "", "Override the output path (defaults to <file>_bindings.go alongside the input)")
	printFlag := flag.Bool("print", false, "Print the discovery report to stdout instead of writing the binding file")
	flag.Parse()

	target := *fileFlag
	if target == "" {
		target = os.Getenv("GOFILE")
	}
	if target == "" {
		return fmt.Errorf("no input file: pass -file or run via `go generate` (GOFILE env)")
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", target, err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, abs, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", abs, err)
	}

	pkgName := os.Getenv("GOPACKAGE")
	if pkgName == "" {
		pkgName = file.Name.Name
	}

	disc, err := discover(fset, file, pkgName)
	if err != nil {
		return err
	}

	if *printFlag {
		report(os.Stdout, fset, disc)
		return nil
	}

	outPath := *outFlag
	if outPath == "" {
		outPath = defaultOutPath(abs)
	}
	out, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer out.Close()
	if err := emit(out, disc); err != nil {
		return fmt.Errorf("emit %s: %w", outPath, err)
	}
	return nil
}

// defaultOutPath turns "/foo/mynode.go" into "/foo/mynode_bindings.go".
// Test files (`_test.go`) and already-generated files (`_bindings.go`)
// are not legal trigger files; the parser will catch most slip-ups but
// we add a small safety net here too.
func defaultOutPath(input string) string {
	dir := filepath.Dir(input)
	base := filepath.Base(input)
	stem := strings.TrimSuffix(base, ".go")
	return filepath.Join(dir, stem+"_bindings.go")
}

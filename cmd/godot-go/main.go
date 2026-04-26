// godot-go is the user-facing codegen tool: when invoked via
// `//go:generate godot-go`, it reads the trigger file (named by the
// GOFILE env var that go generate sets), discovers the user's extension
// class declaration, and — in later phases — emits a sibling
// <file>_bindings.go with the GDExtension class-registration glue.
//
// Phase 5b scope: discovery only. The tool parses GOFILE, identifies the
// main class, base class, inner classes, enums, and methods, and prints a
// human-readable summary. No file is written. Phase 5c will fold in the
// emitter.
//
// The tool can also be run standalone for development/debugging:
//
//	go run ./cmd/godot-go -file path/to/mynode.go
package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "godot-go:", err)
		os.Exit(1)
	}
}

func run() error {
	fileFlag := flag.String("file", "", "Override the trigger file (defaults to $GOFILE)")
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

	report(os.Stdout, fset, disc)
	return nil
}

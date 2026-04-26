package main

import (
	"fmt"
	"go/token"
	"io"
)

// report writes a human-readable summary of the discovery to w. Format is
// Phase 5b's exit gate — once the printed shape matches expectations
// against examples/smoke/mynode.go, we move on to Phase 5c (emitting the
// equivalent registration code).
func report(w io.Writer, fset *token.FileSet, d *discovered) {
	fmt.Fprintf(w, "godot-go: discovered %s in package %q (%s)\n",
		filename(fset, d.MainClass.Pos), d.PackageName, d.FilePath)

	c := d.MainClass
	fmt.Fprintf(w, "  class %s\n", c.Name)
	fmt.Fprintf(w, "    parent:      %s.%s\n", lastSegment(c.ParentImport), c.Parent)
	if c.Description != "" {
		fmt.Fprintf(w, "    description: %s\n", c.Description)
	}
	if c.IsAbstract {
		fmt.Fprintln(w, "    abstract:    true")
	}
	reportMethods(w, c.Methods)

	for _, ic := range d.InnerClasses {
		fmt.Fprintf(w, "  inner class %s\n", ic.Name)
		if ic.Description != "" {
			fmt.Fprintf(w, "    description: %s\n", ic.Description)
		}
		reportMethods(w, ic.Methods)
	}

	for _, e := range d.Enums {
		fmt.Fprintf(w, "  enum %s (%d values)\n", e.Name, len(e.Values))
		for _, v := range e.Values {
			fmt.Fprintf(w, "    %s\n", v.Name)
		}
	}
}

func reportMethods(w io.Writer, methods []*methodInfo) {
	if len(methods) == 0 {
		fmt.Fprintln(w, "    methods: (none)")
		return
	}
	fmt.Fprintln(w, "    methods:")
	for _, m := range methods {
		recv := "value"
		if m.IsPointerRcv {
			recv = "pointer"
		}
		fmt.Fprintf(w, "      %s -> %s [%s, %s recv]\n", m.GoName, m.GodotName, m.Kind, recv)
	}
}

func filename(fset *token.FileSet, pos token.Pos) string {
	return fset.Position(pos).Filename
}

func lastSegment(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

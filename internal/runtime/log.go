// Package runtime provides Go-friendly helpers layered on top of the raw
// gdextension cgo bindings. It exists so user code (and generated code)
// doesn't have to think about caller info, formatting, or C-string
// conversion when logging.
package runtime

import (
	"fmt"
	goruntime "runtime"

	"github.com/legendary-code/godot-go/internal/gdextension"
)

// Print logs the formatted args to Godot as a warning. Caller info (function
// name, file, line) is captured automatically so the message is traceable in
// the editor's Output dock.
func Print(args ...any) {
	fn, file, line := callerInfo(2)
	gdextension.PrintWarning(fmt.Sprint(args...), fn, file, line, false)
}

// Printf is Print with a format string.
func Printf(format string, args ...any) {
	fn, file, line := callerInfo(2)
	gdextension.PrintWarning(fmt.Sprintf(format, args...), fn, file, line, false)
}

// Printerr logs the formatted args as an error.
func Printerr(args ...any) {
	fn, file, line := callerInfo(2)
	gdextension.PrintError(fmt.Sprint(args...), fn, file, line, false)
}

// Printerrf is Printerr with a format string.
func Printerrf(format string, args ...any) {
	fn, file, line := callerInfo(2)
	gdextension.PrintError(fmt.Sprintf(format, args...), fn, file, line, false)
}

func callerInfo(skip int) (fn, file string, line int32) {
	pc, f, l, ok := goruntime.Caller(skip)
	if !ok {
		return "<unknown>", "<unknown>", 0
	}
	name := "<unknown>"
	if fp := goruntime.FuncForPC(pc); fp != nil {
		name = fp.Name()
	}
	return name, f, int32(l)
}

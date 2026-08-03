package src

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// === Diagnostics ===
//
// This file implements Tinoc's shared diagnostic engine, used by every
// compiler stage (lexer, parser, sema, codegen) that needs to report a
// problem tied to a source position.
//
// Design goal ("Go style but colorful"): mirror the terse, single-line,
// no-decoration format Go's own toolchain uses --
//
//	./main.go:6:9: undefined: foo
//
// -- but colorize the pieces that matter (the "error"/"warning" tag, the
// position, and the message) when the terminal supports it. No boxes, no
// carets/underlines, no multi-line source snippets: minimal and fast to
// scan, same as `go build` output, just with color.

// Severity classifies a diagnostic.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
	SeverityNote
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityNote:
		return "note"
	default:
		return "error"
	}
}

func (s Severity) color() string {
	switch s {
	case SeverityError:
		return colorRed
	case SeverityWarning:
		return colorYellow
	case SeverityNote:
		return colorCyan
	default:
		return colorRed
	}
}

// Diagnostic is a single positioned compiler message. Stage identifies
// which phase raised it (lexer/parser/sema/codegen) purely for grouping in
// summaries; it is not printed as part of the Go-style line itself.
type Diagnostic struct {
	Severity Severity
	Stage    string
	File     string
	Line     int
	Column   int
	Message  string
}

// String renders the diagnostic the way `go build` renders its own:
//
//	file:line:col: message
//
// A leading severity tag is only added for warnings/notes (Go's own
// compiler only ever emits errors on this path, so plain errors stay
// tag-free to match it exactly); this keeps error output byte-for-byte
// familiar while still letting sema/codegen surface non-fatal warnings
// clearly.
func (d *Diagnostic) String() string {
	pos := fmt.Sprintf("%s:%d:%d", posOrStdin(d.File), d.Line, d.Column)
	if d.Severity == SeverityError {
		return fmt.Sprintf("%s: %s", pos, d.Message)
	}
	return fmt.Sprintf("%s: %s: %s", pos, d.Severity.String(), d.Message)
}

func posOrStdin(file string) string {
	if file == "" {
		return "<input>"
	}
	return file
}

// Colorize renders the diagnostic with Go-style layout plus color: the
// position is dimmed (like Go's tool output when piped through common
// colorized wrappers), the severity tag (when present) is colored per its
// kind, and the message stays plain so it's easy to read. No underlines,
// no boxes, no source snippet -- minimal, matching Go's own austerity.
func (d *Diagnostic) Colorize(useColor bool) string {
	if !useColor {
		return d.String()
	}
	pos := fmt.Sprintf("%s:%d:%d", posOrStdin(d.File), d.Line, d.Column)
	c := d.Severity.color()

	if d.Severity == SeverityError {
		return fmt.Sprintf("%s%s%s: %s%s%s",
			colorDim, pos, colorReset,
			c, d.Message, colorReset,
		)
	}
	return fmt.Sprintf("%s%s%s: %s%s%s%s: %s",
		colorDim, pos, colorReset,
		c, d.Severity.String(), colorReset,
		":", d.Message,
	)
}

// Diagnostics is an ordered collection of Diagnostic, with helpers for
// building it up during a compiler stage and reporting it afterward.
type Diagnostics struct {
	File  string // default file path stamped on entries that don't set one
	items []*Diagnostic
}

// NewDiagnostics creates an empty diagnostic collector for the given file.
func NewDiagnostics(file string) *Diagnostics {
	return &Diagnostics{File: file}
}

func (d *Diagnostics) add(sev Severity, stage string, line, col int, format string, args ...interface{}) {
	d.items = append(d.items, &Diagnostic{
		Severity: sev,
		Stage:    stage,
		File:     d.File,
		Line:     line,
		Column:   col,
		Message:  fmt.Sprintf(format, args...),
	})
}

// Error records an error-level diagnostic at the given position.
func (d *Diagnostics) Error(stage string, line, col int, format string, args ...interface{}) {
	d.add(SeverityError, stage, line, col, format, args...)
}

// Warn records a warning-level diagnostic at the given position.
func (d *Diagnostics) Warn(stage string, line, col int, format string, args ...interface{}) {
	d.add(SeverityWarning, stage, line, col, format, args...)
}

// Note records a note-level diagnostic at the given position.
func (d *Diagnostics) Note(stage string, line, col int, format string, args ...interface{}) {
	d.add(SeverityNote, stage, line, col, format, args...)
}

// All returns every diagnostic recorded so far, in the order they were
// added.
func (d *Diagnostics) All() []*Diagnostic { return d.items }

// HasErrors reports whether any error-level diagnostic was recorded.
func (d *Diagnostics) HasErrors() bool {
	for _, item := range d.items {
		if item.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Count returns the number of diagnostics at or above the given severity's
// "seriousness" -- specifically the exact-severity count, since callers
// generally want error count and warning count separately.
func (d *Diagnostics) Count(sev Severity) int {
	n := 0
	for _, item := range d.items {
		if item.Severity == sev {
			n++
		}
	}
	return n
}

// Merge appends another Diagnostics collection's items onto this one,
// preserving order. Used to fold sema diagnostics gathered per-function
// back into a single top-level collector.
func (d *Diagnostics) Merge(other *Diagnostics) {
	if other == nil {
		return
	}
	d.items = append(d.items, other.items...)
}

// Print writes every diagnostic to w, one per line, Go-style. Errors and
// warnings are interleaved in the order they were recorded (matching how
// `go build` reports them as it encounters them), colorized when useColor
// is set. Stops and returns the first write error encountered (e.g. a
// closed pipe), matching the behavior of fmt.Fprint family functions
// elsewhere in the standard library.
func (d *Diagnostics) Print(w io.Writer, useColor bool) error {
	for _, item := range d.items {
		if _, err := fmt.Fprintln(w, item.Colorize(useColor)); err != nil {
			return err
		}
	}
	return nil
}

// PrintStderr is a convenience wrapper for the common case of printing to
// os.Stderr with the current terminal's color support auto-detected. A
// write failure to stderr itself isn't actionable for the caller (there
// is nowhere better to report it), so it's discarded here explicitly
// rather than propagated.
func (d *Diagnostics) PrintStderr() {
	_ = d.Print(os.Stderr, supportsColor())
}

// Summary renders a single trailing line summarizing error/warning counts,
// e.g. "3 errors" or "1 error, 2 warnings" -- matching the terse tallies
// Go-family tools print after a failed build, colorized to match.
func (d *Diagnostics) Summary(useColor bool) string {
	errs := d.Count(SeverityError)
	warns := d.Count(SeverityWarning)

	var parts []string
	if errs > 0 {
		label := pluralize(errs, "error", "errors")
		if useColor {
			parts = append(parts, fmt.Sprintf("%s%s%s", colorRed, label, colorReset))
		} else {
			parts = append(parts, label)
		}
	}
	if warns > 0 {
		label := pluralize(warns, "warning", "warnings")
		if useColor {
			parts = append(parts, fmt.Sprintf("%s%s%s", colorYellow, label, colorReset))
		} else {
			parts = append(parts, label)
		}
	}
	if len(parts) == 0 {
		return "no issues"
	}
	return strings.Join(parts, ", ")
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// diagFromParseError adapts a *ParseError (parser.go's own lightweight
// error type) into a *Diagnostic so the parser's errors can flow through
// the same collection, printing, and summary machinery as sema/codegen
// diagnostics.
func diagFromParseError(file string, e *ParseError) *Diagnostic {
	return &Diagnostic{
		Severity: SeverityError,
		Stage:    "parser",
		File:     file,
		Line:     e.Line,
		Column:   e.Column,
		Message:  e.Message,
	}
}

package src

import "core:fmt"
import "core:strings"

// Pos is a single location in a source file (1-indexed line/column).
Pos :: struct {
	line: int,
	col:  int,
}

// Severity classifies a diagnostic message.
Severity :: enum {
	Error,
	Warning,
	Note,
}

// Diagnostic is a single compiler message tied to a source location.
Diagnostic :: struct {
	severity: Severity,
	pos:      Pos,
	message:  string,
}

// Diagnostics collects messages produced during a single compiler run.
Diagnostics :: struct {
	items: [dynamic]Diagnostic,
}

diagnostics_make :: proc() -> Diagnostics {
	return Diagnostics{items = make([dynamic]Diagnostic)}
}

diagnostics_destroy :: proc(d: ^Diagnostics) {
	delete(d.items)
}

diag_add :: proc(d: ^Diagnostics, severity: Severity, pos: Pos, fmt_str: string, args: ..any) {
	msg := fmt.aprintf(fmt_str, ..args)
	append(&d.items, Diagnostic{severity = severity, pos = pos, message = msg})
}

diag_error :: proc(d: ^Diagnostics, pos: Pos, fmt_str: string, args: ..any) {
	diag_add(d, .Error, pos, fmt_str, ..args)
}

diag_warning :: proc(d: ^Diagnostics, pos: Pos, fmt_str: string, args: ..any) {
	diag_add(d, .Warning, pos, fmt_str, ..args)
}

has_errors :: proc(d: ^Diagnostics) -> bool {
	for item in d.items {
		if item.severity == .Error {
			return true
		}
	}
	return false
}

error_count :: proc(d: ^Diagnostics) -> int {
	count := 0
	for item in d.items {
		if item.severity == .Error {
			count += 1
		}
	}
	return count
}

severity_label :: proc(s: Severity) -> string {
	switch s {
	case .Error:
		return "error"
	case .Warning:
		return "warning"
	case .Note:
		return "note"
	}
	return "info"
}

// source_line extracts the raw text of a single 1-indexed line from source.
source_line :: proc(source: string, line: int) -> string {
	current := 1
	start := 0
	for i := 0; i < len(source); i += 1 {
		if current == line {
			start = i
			for i < len(source) && source[i] != '\n' {
				i += 1
			}
			return source[start:i]
		}
		if source[i] == '\n' {
			current += 1
		}
	}
	if current == line {
		return source[start:]
	}
	return ""
}

// print_diagnostics renders every collected diagnostic with a source snippet and caret.
print_diagnostics :: proc(d: ^Diagnostics, file_path, source: string, use_color: bool) {
	for item in d.items {
		print_diagnostic(item, file_path, source, use_color)
	}
}

print_diagnostic :: proc(item: Diagnostic, file_path, source: string, use_color: bool) {
	label := severity_label(item.severity)
	color := COLOR_RED
	if item.severity == .Warning {
		color = COLOR_YELLOW
	} else if item.severity == .Note {
		color = COLOR_CYAN
	}

	if use_color {
		fmt.eprintf(
			"%s%s%s: %s %s[%s:%d:%d]%s\n",
			color,
			label,
			COLOR_RESET,
			item.message,
			COLOR_DIM,
			file_path,
			item.pos.line,
			item.pos.col,
			COLOR_RESET,
		)
	} else {
		fmt.eprintf("%s: %s [%s:%d:%d]\n", label, item.message, file_path, item.pos.line, item.pos.col)
	}

	line_text := source_line(source, item.pos.line)
	if line_text == "" {
		return
	}

	line_num_str := fmt.tprintf("%d", item.pos.line)
	gutter := strings.repeat(" ", len(line_num_str))

	if use_color {
		fmt.eprintf("  %s%s |%s\n", COLOR_DIM, gutter, COLOR_RESET)
		fmt.eprintf("  %s%s%s | %s%s%s\n", COLOR_DIM, line_num_str, COLOR_RESET, "", line_text, "")
	} else {
		fmt.eprintf("  %s |\n", gutter)
		fmt.eprintf("  %s | %s\n", line_num_str, line_text)
	}

	caret_col := item.pos.col - 1
	if caret_col < 0 {
		caret_col = 0
	}
	caret_pad := strings.repeat(" ", caret_col)
	if use_color {
		fmt.eprintf("  %s |%s %s%s^%s\n", gutter, COLOR_RESET, caret_pad, color, COLOR_RESET)
	} else {
		fmt.eprintf("  %s | %s^\n", gutter, caret_pad)
	}
}

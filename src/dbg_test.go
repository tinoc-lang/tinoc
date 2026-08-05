package src

import (
	"strings"
	"testing"
)

func TestDbgAuxParse(t *testing.T) {
	line := "/* /usr/include/stdio.h:356:NC */ extern int printf (const char *, ...);"
	t.Logf("prefix extern? %v", strings.HasPrefix(line, "extern "))
	if idx := strings.Index(line, "*/ "); idx >= 0 {
		line = strings.TrimSpace(line[idx+3:])
	}
	t.Logf("after strip: %q", line)
	rest := strings.TrimSpace(strings.TrimPrefix(line, "extern "))
	rest = strings.TrimSuffix(rest, ";")
	rest = stripCAttributes(rest)
	t.Logf("rest: %q", rest)
	name, ret, params, variadic, ok := parseAuxSignature(rest)
	t.Logf("ok=%v name=%q ret=%v params=%d variadic=%v", ok, name, ret, len(params), variadic)
	for i, p := range params {
		t.Logf("  param %d: %v (%s)", i, p, p.CType())
	}
}

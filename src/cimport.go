package src

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// === C Header Importing (#importc) ===
//
// `#importc "stdio.h" as cio;` asks Tinoc to parse a C header and expose
// every function, extern variable, enum constant, typedef, and simple
// macro through the alias namespace, so `cio.printf(...)` is fully type
// checked at comptime and still compiles to a plain C call (codegen emits
// `#include <stdio.h>`).
//
// Two backends, chosen by smart compiler detection:
//
//  1. clang (preferred): `-Xclang -ast-dump=json -fsyntax-only` yields a
//     complete JSON AST of the whole translation unit, including the
//     requested header and everything it pulls in. This gives 100%
//     awareness: functions, params, variadics, typedefs, struct/enum
//     tags, extern globals, enum constants — plus `-dM -E` for macros.
//  2. gcc (fallback): gcc has no JSON AST dump, but its decades-old
//     `-aux-info` flag emits one clean, parseable prototype per line
//     (`extern int printf (const char *, ...);`) for every function
//     declared in the translation unit, and `-dM -E` supplies macros.
//     Structs/typedefs/enum constants are not recovered by this path —
//     a documented limitation, not a silent miscompile (the C compiler
//     still resolves them via the emitted #include).
//
// Parsed results are cached on disk (keyed on the header text, tool path,
// and tool version) so repeat builds don't re-spawn the subprocess.

// CImportModule is the parsed symbol surface of one or more C headers,
// exposed under a single alias (e.g. `cio`).
type CImportModule struct {
	Alias   string
	Headers []string
	Funcs   map[string]*Symbol // C function name -> signature symbol
	Consts  map[string]*Symbol // extern vars + enum constants + simple macros
	Types   map[string]*Type   // typedef / struct / enum tags
}

func newCImportModule(alias string, headers []string) *CImportModule {
	return &CImportModule{
		Alias:   alias,
		Headers: headers,
		Funcs:   make(map[string]*Symbol),
		Consts:  make(map[string]*Symbol),
		Types:   make(map[string]*Type),
	}
}

// addFunc registers a C function signature into the module.
func (m *CImportModule) addFunc(name string, params []*Type, paramNames []string, ret *Type, variadic bool) {
	if name == "" || params == nil {
		params = []*Type{}
	}
	if _, dup := m.Funcs[name]; dup {
		return
	}
	m.Funcs[name] = &Symbol{
		Name:       name,
		Kind:       SymFunc,
		IsCImport:  true,
		CSymbol:    name,
		Params:     params,
		ParamNames: paramNames,
		ReturnType: ret,
		Variadic:   variadic,
	}
}

// addConst registers a read-only C value (extern var / enum constant /
// simple macro) into the module.
func (m *CImportModule) addConst(name string, t *Type) {
	if name == "" {
		return
	}
	if _, dup := m.Consts[name]; dup {
		return
	}
	m.Consts[name] = &Symbol{Name: name, Kind: SymConst, Type: t, Mutable: false, CSymbol: name, IsCImport: true}
}

// === Compiler Detection ===

// headerDumper describes the discovered tool able to extract declarations
// from C headers.
type headerDumper struct {
	Path    string
	Kind    string // "clang" or "gcc"
	Version string
}

var cachedHeaderDumper *headerDumper

// findHeaderDumper probes for a usable header parser: clang is preferred
// (rich JSON AST), gcc is the fallback (aux-info). Each candidate is
// smoke-tested so a broken install falls through to the next option.
func findHeaderDumper() (*headerDumper, error) {
	if cachedHeaderDumper != nil {
		return cachedHeaderDumper, nil
	}

	if ref := os.Getenv("TINOC_HD"); ref != "" {
		d := resolveDumperRef(ref)
		if d != nil {
			cachedHeaderDumper = d
			return d, nil
		}
		return nil, fmt.Errorf("TINOC_HD=%q: not a working C header parser", ref)
	}

	candidates := []string{"clang", "clang-19", "clang-18", "clang-17", "clang-16", "clang-15", "clang-14", "gcc", "cc"}
	var tried []string
	for _, name := range candidates {
		path, err := lookPathManual(name)
		if err != nil {
			continue
		}
		kind := "gcc"
		if strings.Contains(name, "clang") {
			kind = "clang"
		}
		d := &headerDumper{Path: path, Kind: kind, Version: dumperVersion(path)}
		tried = append(tried, name)
		if smokeTestDumper(d) {
			cachedHeaderDumper = d
			return d, nil
		}
	}

	return nil, fmt.Errorf("cannot parse C headers for #importc: no working clang or gcc found on $PATH (tried: %s) — install clang or gcc", strings.Join(tried, ", "))
}

func resolveDumperRef(ref string) *headerDumper {
	path, err := lookPathManual(ref)
	if err != nil {
		if strings.ContainsAny(ref, "/\\") {
			if _, statErr := os.Stat(ref); statErr == nil {
				path = ref
			} else {
				return nil
			}
		} else {
			return nil
		}
	}
	kind := "gcc"
	if strings.Contains(filepath.Base(path), "clang") {
		kind = "clang"
	}
	d := &headerDumper{Path: path, Kind: kind, Version: dumperVersion(path)}
	if smokeTestDumper(d) {
		return d
	}
	return nil
}

func dumperVersion(path string) string {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return "unknown"
	}
	text := string(out)
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	return strings.TrimSpace(text)
}

// smokeTestDumper runs the candidate against a trivial wrapper to prove it
// can actually dump declarations.
func smokeTestDumper(d *headerDumper) bool {
	dir, err := os.MkdirTemp("", "tinoc-hdprobe-*")
	if err != nil {
		return false
	}
	defer os.RemoveAll(dir)
	wrapper := filepath.Join(dir, "wrap.c")
	if err := os.WriteFile(wrapper, []byte("#include <stddef.h>\n"), 0o644); err != nil {
		return false
	}
	if d.Kind == "clang" {
		cmd := exec.Command(d.Path, "-Xclang", "-ast-dump=json", "-fsyntax-only", wrapper)
		return cmd.Run() == nil
	}
	aux := filepath.Join(dir, "out.txt")
	cmd := exec.Command(d.Path, "-aux-info", aux, "-fsyntax-only", wrapper)
	if cmd.Run() != nil {
		return false
	}
	_, err = os.ReadFile(aux)
	return err == nil
}

// === Main Entry ===

// ImportCHeaders parses the given C headers and returns a CImportModule
// exposing their declarations. sourceDir is the directory of the .tnc
// file, used to resolve local header includes; pass "" when unknown.
func ImportCHeaders(alias string, headers []string, sourceDir string) (*CImportModule, error) {
	dumper, err := findHeaderDumper()
	if err != nil {
		return nil, err
	}

	mod := newCImportModule(alias, headers)
	workDir, err := os.MkdirTemp("", "tinoc-cimport-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workDir)

	wrapperSrc := cWrapperSource(headers, sourceDir)
	wrapper := filepath.Join(workDir, "wrap.c")
	if err := os.WriteFile(wrapper, []byte(wrapperSrc), 0o644); err != nil {
		return nil, err
	}

	keyHash := cacheHash(wrapperSrc, dumper)

	// Macros: both backends use -dM -E.
	macroText, err := cachedToolOutput(keyHash+".macros", func() ([]byte, error) {
		return runCapture(dumper.Path, dumperMacroArgs(wrapper, sourceDir))
	})
	if err != nil {
		return nil, fmt.Errorf("#importc %s: macro dump failed: %w", headers[0], err)
	}
	parseMacros(string(macroText), mod)

	// Declaration dump.
	switch dumper.Kind {
	case "clang":
		jsonText, err := cachedToolOutput(keyHash+".dump", func() ([]byte, error) {
			return runCapture(dumper.Path, clangASTArgs(wrapper, sourceDir))
		})
		if err != nil {
			return nil, fmt.Errorf("#importc %s: clang AST dump failed: %w", headers[0], err)
		}
		if err := parseClangJSON(jsonText, mod); err != nil {
			return nil, fmt.Errorf("#importc %s: %w", headers[0], err)
		}
	default: // gcc
		auxText, err := cachedToolOutput(keyHash+".dump", func() ([]byte, error) {
			aux := filepath.Join(workDir, "aux.txt")
			if err := runTool(dumper.Path, gccAuxArgs(aux, wrapper, sourceDir)); err != nil {
				return nil, err
			}
			return os.ReadFile(aux)
		})
		if err != nil {
			return nil, fmt.Errorf("#importc %s: gcc -aux-info failed: %w", headers[0], err)
		}
		parseAuxInfo(string(auxText), mod)
	}

	return mod, nil
}

// === Wrapper / Args ===

// cWrapperSource builds the translation unit that includes the requested
// headers. Angle brackets are used for system headers (bare names that
// don't resolve to a file next to the source); quotes for local paths.
func cWrapperSource(headers []string, sourceDir string) string {
	var b strings.Builder
	for _, h := range headers {
		b.WriteString(cIncludeDirective(h, sourceDir))
		b.WriteString("\n")
	}
	return b.String()
}

// cIncludeDirective renders the `#include <...>` / `#include "..."` line
// for one header. Codegen uses the same helper so the emitted C and the
// parse wrapper always agree on the spelling.
func cIncludeDirective(header, sourceDir string) string {
	if strings.ContainsAny(header, "/\\") || strings.HasPrefix(header, ".") {
		return fmt.Sprintf("#include %q", header)
	}
	if sourceDir != "" {
		if _, err := os.Stat(filepath.Join(sourceDir, header)); err == nil {
			return fmt.Sprintf("#include %q", header)
		}
	}
	return fmt.Sprintf("#include <%s>", header)
}

func includeDirArgs(sourceDir string) []string {
	if sourceDir == "" {
		return nil
	}
	return []string{"-I", sourceDir}
}

func clangASTArgs(wrapper, sourceDir string) []string {
	args := []string{"-Xclang", "-ast-dump=json", "-fsyntax-only", "-std=gnu11"}
	args = append(args, includeDirArgs(sourceDir)...)
	return append(args, wrapper)
}

func gccAuxArgs(auxPath, wrapper, sourceDir string) []string {
	args := []string{"-aux-info", auxPath, "-fsyntax-only"}
	args = append(args, includeDirArgs(sourceDir)...)
	return append(args, wrapper)
}

func dumperMacroArgs(wrapper, sourceDir string) []string {
	args := []string{"-dM", "-E"}
	args = append(args, includeDirArgs(sourceDir)...)
	return append(args, wrapper)
}

func runCapture(path string, args []string) ([]byte, error) {
	cmd := exec.Command(path, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Surface the tool's own diagnostics (clang/gcc errors go to
		// stderr) so a failed header dump isn't an opaque "exit status 1".
		return nil, fmt.Errorf("%s %s: %w: %s", path, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func runTool(path string, args []string) error {
	cmd := exec.Command(path, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", path, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// === Disk Cache ===

func cacheHash(wrapperSrc string, d *headerDumper) string {
	sum := sha256.Sum256([]byte(wrapperSrc + "\x00" + d.Path + "\x00" + d.Version + "\x00" + d.Kind))
	return hex.EncodeToString(sum[:])[:32]
}

// cachedToolOutput serves a subprocess's stdout from a disk cache keyed by
// the wrapper text and tool identity. Cache dir is $TINOC_CACHE_DIR, or
// `.tinoc_cache` under the current directory.
func cachedToolOutput(key string, fn func() ([]byte, error)) ([]byte, error) {
	dir := os.Getenv("TINOC_CACHE_DIR")
	if dir == "" {
		dir = ".tinoc_cache"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fn()
	}
	path := filepath.Join(dir, key)
	if data, err := os.ReadFile(path); err == nil {
		return data, nil
	}
	data, err := fn()
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(path, data, 0o644)
	return data, nil
}

// === Macros ===

// parseMacros extracts simple object-like macros (integer or string
// constants) from `-dM -E` output into module constants. Function-like
// macros and anything not a plain literal are skipped — they have no
// meaningful Tinoc type.
func parseMacros(text string, mod *CImportModule) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#define ") {
			continue
		}
		rest := strings.TrimPrefix(line, "#define ")
		sp := strings.IndexAny(rest, " \t")
		if sp < 0 {
			continue
		}
		name, value := rest[:sp], strings.TrimSpace(rest[sp+1:])
		if name == "" || strings.HasPrefix(name, "__") || strings.Contains(name, "(") {
			continue
		}
		value = strings.TrimSpace(strings.Trim(value, "()"))
		if iv, ok := parseMacroInt(value); ok {
			_ = iv
			mod.addConst(name, typeI32)
		} else if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			mod.addConst(name, typeStr)
		}
	}
}

// parseMacroInt parses a C integer macro value (optional u/U/l/L suffix,
// optional parentheses, decimal/hex/octal), returning the value.
func parseMacroInt(v string) (int64, bool) {
	v = strings.TrimSpace(v)
	for len(v) > 0 {
		c := v[len(v)-1]
		if c == 'u' || c == 'U' || c == 'l' || c == 'L' {
			v = v[:len(v)-1]
			continue
		}
		break
	}
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 0, 64)
	if err != nil {
		// Also accept values that overflow int64 but parse as uint64.
		u, uerr := strconv.ParseUint(v, 0, 64)
		if uerr != nil {
			return 0, false
		}
		return int64(u), true
	}
	return n, true
}

// === clang JSON AST Path ===
//
// clangNode mirrors the subset of clang's `-ast-dump=json` schema Tinoc
// consumes. The root is a TranslationUnitDecl whose `inner` array holds
// every top-level declaration from the translation unit (the requested
// header plus everything it pulls in).

type clangNode struct {
	ID           string        `json:"id"`
	Kind         string        `json:"kind"`
	Name         string        `json:"name"`
	TagUsed      string        `json:"tagUsed"`
	StorageClass string        `json:"storageClass"`
	Variadic     bool          `json:"variadic"`
	Type         *clangTypeRef `json:"type"`
	Inner        []clangNode   `json:"inner"`
}

type clangTypeRef struct {
	QualType string `json:"qualType"`
}

// parseClangJSON decodes a clang AST JSON dump and populates the module.
func parseClangJSON(text []byte, mod *CImportModule) error {
	var root clangNode
	if err := json.Unmarshal(text, &root); err != nil {
		return fmt.Errorf("cannot parse clang AST JSON: %w", err)
	}
	walkClangNode(&root, mod)
	return nil
}

// walkClangNode recursively visits a clang AST node, importing the decl
// kinds that make sense for Tinoc and recursing into children (params,
// enum constants, nested decls).
func walkClangNode(n *clangNode, mod *CImportModule) {
	switch n.Kind {
	case "FunctionDecl":
		mod.addClangFunc(n)
	case "TypedefDecl":
		if n.Name != "" && n.Type != nil {
			if _, dup := mod.Types[n.Name]; !dup {
				mod.Types[n.Name] = cTypeFromQualType(n.Type.QualType)
			}
		}
	case "RecordDecl":
		if n.Name != "" {
			tag := n.TagUsed
			if tag == "" {
				tag = "struct"
			}
			name := tag + " " + n.Name
			if _, dup := mod.Types[name]; !dup {
				mod.Types[name] = &Type{Kind: KindUnknown, Name: name}
			}
		}
	case "EnumDecl":
		if n.Name != "" {
			name := "enum " + n.Name
			if _, dup := mod.Types[name]; !dup {
				mod.Types[name] = &Type{Kind: KindUnknown, Name: name}
			}
		}
	case "EnumConstantDecl":
		if n.Name != "" {
			t := typeI32
			if n.Type != nil {
				t = cTypeFromQualType(n.Type.QualType)
			}
			mod.addConst(n.Name, t)
		}
	case "VarDecl":
		// Extern globals (glibc's stdin/stdout/stderr, errno, environ, ...)
		// become read-only module constants.
		if n.Name != "" && n.Type != nil &&
			(n.StorageClass == "extern" || n.Name == "stdin" || n.Name == "stdout" || n.Name == "stderr") {
			mod.addConst(n.Name, cTypeFromQualType(n.Type.QualType))
		}
	}
	for i := range n.Inner {
		walkClangNode(&n.Inner[i], mod)
	}
}

// addClangFunc imports a clang FunctionDecl. The return type comes from
// the qualType's prefix ("int (const char *, ...)" -> "int"); params come
// from ParmVarDecl children, with a qualType-paren fallback; variadic is
// detected from the node's own field (newer clang) or a trailing
// ", ...)" in the qualType (all versions).
func (m *CImportModule) addClangFunc(n *clangNode) {
	if n.Name == "" {
		return
	}
	qt := ""
	if n.Type != nil {
		qt = n.Type.QualType
	}
	retStr := qt
	if idx := strings.Index(qt, "("); idx >= 0 {
		retStr = strings.TrimSpace(qt[:idx])
	}

	var params []*Type
	var paramNames []string
	for i := range n.Inner {
		c := &n.Inner[i]
		if c.Kind == "ParmVarDecl" && c.Type != nil {
			params = append(params, cTypeFromQualType(c.Type.QualType))
			paramNames = append(paramNames, c.Name)
		}
	}

	variadic := n.Variadic || strings.Contains(qt, ", ...)")

	if len(params) == 0 && strings.Contains(qt, "(") {
		open := strings.Index(qt, "(")
		close := strings.LastIndex(qt, ")")
		if close > open {
			parseCParamList(qt[open+1:close], &params, &paramNames, &variadic)
		}
	}

	ret := cTypeFromQualType(retStr)

	// A lone "void" parameter is C's (void) — no parameters at all.
	if len(params) == 1 && params[0].Kind == KindVoid {
		params = nil
		paramNames = nil
	}

	m.addFunc(n.Name, params, paramNames, ret, variadic)
}

// === gcc -aux-info Path ===

// parseAuxInfo parses gcc -aux-info output. Every function declaration is
// one line of the form:
//
//	/* /usr/include/stdio.h:356:NC */ extern int printf (const char *, ...);
func parseAuxInfo(text string, mod *CImportModule) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "*/ "); idx >= 0 {
			line = strings.TrimSpace(line[idx+3:])
		}
		if !strings.HasPrefix(line, "extern ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "extern "))
		rest = strings.TrimSuffix(rest, ";")
		rest = stripCAttributes(rest)
		if name, ret, params, variadic, ok := parseAuxSignature(rest); ok {
			mod.addFunc(name, params, nil, ret, variadic)
		}
	}
}

// stripCAttributes removes `__attribute__((...))` / `__asm__(...)`
// fragments that some headers attach to declarations.
func stripCAttributes(s string) string {
	for {
		idx := strings.Index(s, "__attribute__")
		if idx < 0 {
			idx = strings.Index(s, "__asm__")
		}
		if idx < 0 {
			return strings.TrimSpace(s)
		}
		// Find the matching close paren of the attribute's first '('.
		depth := 0
		close := -1
		for i := idx; i < len(s); i++ {
			switch s[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					close = i
					i = len(s)
				}
			}
		}
		if close < 0 {
			return strings.TrimSpace(s)
		}
		s = s[:idx] + s[close+1:]
	}
}

// parseAuxSignature splits the text after "extern " into name, return
// type, parameter types, and the variadic flag. Function-pointer return
// types ("void (*signal(int, ...))(int)") don't have a name before the
// first top-level '(' and are skipped rather than misparsed.
func parseAuxSignature(rest string) (name string, ret *Type, params []*Type, variadic bool, ok bool) {
	open := -1
	depth := 0
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '(':
			if depth == 0 {
				open = i
			}
			depth++
		case ')':
			depth--
			if depth == 0 && open >= 0 {
				name, nameStart := identBefore(rest, open)
				if name == "" {
					return "", nil, nil, false, false
				}
				// The return type is everything before the name's own start
				// (identBefore also skips the whitespace gap before the
				// paren, which a naive open-len(name) slice would leak into
				// the type text: "int printf (" -> "int p" without it).
				ret = cTypeFromQualType(strings.TrimSpace(rest[:nameStart]))
				parseCParamList(rest[open+1:i], &params, nil, &variadic)
				return name, ret, params, variadic, true
			}
		}
	}
	return "", nil, nil, false, false
}

// identBefore returns the identifier immediately preceding position i
// (skipping any whitespace between them, "int printf (" -> "printf") and
// the index where that identifier starts, so callers can slice the text
// before it without leaking the gap.
func identBefore(s string, i int) (string, int) {
	j := i - 1
	for j >= 0 && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
		j--
	}
	end := j + 1
	for j >= 0 && (s[j] == '_' || (s[j] >= 'a' && s[j] <= 'z') || (s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= '0' && s[j] <= '9')) {
		j--
	}
	return s[j+1 : end], j + 1
}

// parseCParamList splits a C parameter list text ("const char *, ...") on
// top-level commas, mapping each parameter type into params. "void" means
// no parameters; a trailing "..." sets variadic.
func parseCParamList(s string, params *[]*Type, paramNames *[]string, variadic *bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "void" {
		return
	}
	for _, part := range splitTopLevel(s, ',') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "..." {
			*variadic = true
			continue
		}
		*params = append(*params, cTypeFromQualType(stripCParamName(part)))
		if paramNames != nil {
			*paramNames = append(*paramNames, "")
		}
	}
}

// stripCParamName removes a trailing parameter name from a C parameter
// text when one is present (some gcc -aux-info outputs include names:
// "const char *__restrict __format" -> "const char *__restrict"). A
// trailing token is only treated as a name when what remains still ends
// in a pointer star or a qualifier; a bare typedef name ("size_t",
// "FILE") is kept intact since there is no way to tell it from a named
// parameter without the typedef table.
func stripCParamName(part string) string {
	tokens := strings.Fields(part)
	if len(tokens) < 2 {
		return part
	}
	last := tokens[len(tokens)-1]
	if !isCIdentifier(last) {
		return part
	}
	rest := strings.TrimSpace(strings.TrimSuffix(part, last))
	if strings.HasSuffix(rest, "*") {
		return rest
	}
	restTokens := strings.Fields(rest)
	if len(restTokens) > 0 {
		switch restTokens[len(restTokens)-1] {
		case "const", "volatile", "restrict", "__restrict", "__restrict__", "__const", "inline", "_Atomic", "_Nonnull", "_Nullable", "_Null_unspecified":
			return rest
		}
	}
	return part
}

func isCIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

// splitTopLevel splits s on sep, ignoring separators nested inside (), [],
// or {} groups.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		default:
			if s[i] == sep && depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

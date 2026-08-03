package src

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// === C Compiler Discovery ===
//
// Tinoc transpiles to C99 and then needs a real C compiler to turn that
// into a binary for `build`/`run`. Systems differ in what's installed
// (gcc, clang, a vendor cc shim, tcc, ...), so this file probes for one
// at pipeline time rather than hardcoding a single name, and reports
// which one it picked so the pipeline output is never ambiguous about
// what actually compiled the program.

// CCompiler describes a discovered C compiler toolchain.
type CCompiler struct {
	// Path is the resolved executable, either from $TINOC_CC or found on
	// $PATH.
	Path string
	// Name is the short display name used in pipeline output, e.g.
	// "gcc", "clang", "cc", "tcc".
	Name string
	// Kind classifies the compiler family so codegen/build flags that
	// differ by vendor (e.g. warning flag spelling) can branch on it.
	Kind CCompilerKind
	// Version is a best-effort one-line version string (first line of
	// `<cc> --version`), used only for -v/--verbose diagnostics.
	Version string
}

// CCompilerKind identifies the compiler family.
type CCompilerKind int

const (
	CCUnknown CCompilerKind = iota
	CCGCC
	CCClang
	CCTinyCC
)

func (k CCompilerKind) String() string {
	switch k {
	case CCGCC:
		return "gcc"
	case CCClang:
		return "clang"
	case CCTinyCC:
		return "tcc"
	default:
		return "unknown"
	}
}

// ccCandidates lists the executable names probed on $PATH, in priority
// order. `cc` is checked first since it's the POSIX-mandated name for
// "the system's C compiler" and is often a symlink already pointing at
// whichever of gcc/clang the platform prefers; gcc and clang are then
// checked directly by name for systems where only one is installed
// without a `cc` alias, and tcc last as a lightweight fallback.
var ccCandidates = []string{"cc", "gcc", "clang", "tcc"}

// lookPathManual searches $PATH for an executable named `name`, without
// going through exec.LookPath. This exists because exec.LookPath (and
// the eaccess check some Go versions use internally) can invoke the
// faccessat2 syscall, which is blocked under some restrictive seccomp/
// sandbox profiles (gVisor, some container runtimes, some CI sandboxes)
// and causes the process to be killed with SIGSYS instead of returning
// an error — there is no way to recover from that in Go once the
// syscall is attempted. Walking $PATH with plain os.Stat avoids that
// code path entirely, at the cost of a slightly less precise
// executable-bit check (os.Stat can't check the exec bit the way
// faccessat2 can; a mode-bit check is what many portable tools already
// fall back to for this reason).
func lookPathManual(name string) (string, error) {
	if strings.ContainsAny(name, "/\\") {
		if info, err := os.Stat(name); err == nil && !info.IsDir() {
			return name, nil
		}
		return "", fmt.Errorf("not found: %s", name)
	}

	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		// Best-effort executable-bit check on POSIX; skipped entirely if
		// the mode bits aren't meaningful on this platform (e.g.
		// Windows), where any regular file match is accepted.
		if info.Mode()&0o111 != 0 || runtime.GOOS == "windows" {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("not found on $PATH: %s", name)
}

// FindCCompiler probes for an available C compiler and returns a
// resolved CCompiler describing it. Resolution order:
//
//  1. $TINOC_CC, if set — an explicit user override naming a compiler
//     executable (by name, resolved via $PATH, or by absolute path).
//  2. $CC, the conventional Unix environment override most build tools
//     already respect.
//  3. cc, gcc, clang, tcc, tried in that order against $PATH.
//
// Returns an error listing everything that was tried if none are found,
// so the caller can surface one clear diagnostic instead of a bare "not
// found".
func FindCCompiler() (*CCompiler, error) {
	if override := os.Getenv("TINOC_CC"); override != "" {
		cc, err := resolveCCompiler(override)
		if err != nil {
			return nil, fmt.Errorf("TINOC_CC=%q: %w", override, err)
		}
		return cc, nil
	}

	if override := os.Getenv("CC"); override != "" {
		cc, err := resolveCCompiler(override)
		if err != nil {
			return nil, fmt.Errorf("CC=%q: %w", override, err)
		}
		return cc, nil
	}

	var tried []string
	for _, name := range ccCandidates {
		tried = append(tried, name)
		if path, err := lookPathManual(name); err == nil {
			return newCCompiler(path, name), nil
		}
	}

	return nil, fmt.Errorf(
		"no C compiler found on $PATH (tried: %s) — install gcc, clang, or tcc, "+
			"or set $CC / $TINOC_CC to point at one",
		strings.Join(tried, ", "),
	)
}

// resolveCCompiler resolves a user-provided compiler reference (a bare
// name to look up on $PATH, or an absolute/relative path to an
// executable) into a CCompiler.
func resolveCCompiler(ref string) (*CCompiler, error) {
	// A path with a separator is used as-is (relative or absolute);
	// otherwise treat it as a $PATH lookup, same as exec.Command would.
	if strings.ContainsAny(ref, "/\\") {
		if info, err := os.Stat(ref); err != nil || info.IsDir() {
			return nil, fmt.Errorf("not an executable file: %s", ref)
		}
		return newCCompiler(ref, filepath.Base(ref)), nil
	}
	path, err := lookPathManual(ref)
	if err != nil {
		return nil, fmt.Errorf("not found on $PATH: %s", ref)
	}
	return newCCompiler(path, ref), nil
}

func newCCompiler(path, invokedAs string) *CCompiler {
	cc := &CCompiler{Path: path, Name: invokedAs}
	cc.Kind, cc.Version = probeCCompiler(path)
	if cc.Kind != CCUnknown {
		// Prefer the detected family's canonical name for display even
		// when invoked via a differently-named symlink (e.g. `cc` that
		// resolves to gcc), so pipeline output says what it actually is.
		cc.Name = cc.Kind.String()
	}
	return cc
}

// probeCCompiler runs `<path> --version` and sniffs the output to
// classify the compiler family and capture a one-line version string.
// Best-effort: an unrecognized or failing probe just yields CCUnknown
// with an empty version rather than an error, since compilation can
// still proceed with generic flags either way.
func probeCCompiler(path string) (CCompilerKind, string) {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return CCUnknown, ""
	}
	text := string(out)
	firstLine := text
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		firstLine = text[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)

	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "clang"):
		return CCClang, firstLine
	case strings.Contains(lower, "tcc") || strings.Contains(lower, "tiny c compiler"):
		return CCTinyCC, firstLine
	case strings.Contains(lower, "gcc") || strings.Contains(lower, "free software foundation"):
		return CCGCC, firstLine
	default:
		return CCUnknown, firstLine
	}
}

// BuildArgs returns the argument list to compile a single C source file
// into an output binary, tuned per compiler family. All three supported
// families accept this same core flag set (-std=c99, -O2, -o); kept as a
// method (rather than one hardcoded slice in cmd.go) so a compiler-
// specific quirk can be special-cased here later without touching the
// pipeline driver.
func (c *CCompiler) BuildArgs(cFile, outPath string, includeDirs []string) []string {
	args := []string{"-std=c99", "-O2"}
	if c.Kind != CCTinyCC {
		// tcc's -Wall/-Wextra coverage is minimal and noisy in ways
		// unrelated to generated-code quality; skip them there, keep
		// them for gcc/clang where they're meaningful signal.
		args = append(args, "-Wall", "-Wextra")
	}
	for _, dir := range includeDirs {
		args = append(args, "-I", dir)
	}
	args = append(args, "-o", outPath, cFile, "-lm")
	return args
}

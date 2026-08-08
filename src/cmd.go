package src

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	CompilerName = "tinoc"

	// Project metadata shown by `tinoc version` and `tinoc help` so the
	// CLI itself points at the repository, creator, website, and license.
	RepoURL       = "https://github.com/tinoc-lang/tinoc"
	CreatorGitHub = "https://github.com/pbarot2009"
	Website       = "https://tinoc-lang.vercel.app"
	LicenseName   = "Apache-2.0"
)

// Version is the compiler version. build.sh / build.ps1 override it at link
// time via -ldflags "-X github.com/tinoc-lang/tinoc/src.Version=<tag>" so
// release binaries report the actual tag; the in-tree default keeps plain
// `go build` / `go run` behaving sensibly.
var Version = "0.1.0"

// Color constants and supportsColor are defined in lexer.go and shared
// across this package.

func stage(useColor bool, label, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if useColor {
		fmt.Printf("%s[%s]%s %s\n", colorCyan, label, colorReset, msg)
	} else {
		fmt.Printf("[%s] %s\n", label, msg)
	}
}

func ok(useColor bool, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if useColor {
		fmt.Printf("  %s%s%s\n", colorGreen, msg, colorReset)
	} else {
		fmt.Printf("  %s\n", msg)
	}
}

func fail(useColor bool, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if useColor {
		fmt.Fprintf(os.Stderr, "%serror:%s %s\n", colorRed, colorReset, msg)
	} else {
		fmt.Fprintf(os.Stderr, "error: %s\n", msg)
	}
}

// PipelineConfig holds flags and parameters for a compiler execution run.
type PipelineConfig struct {
	FilePath   string
	OutputPath string
	Lex        bool
	AST        bool
	EmitC      bool
	Verbose    bool
}

// Execute is the main entry point for CLI subcommand processing.
func Execute(args []string) {
	if len(args) < 1 {
		printGlobalHelp()
		os.Exit(0)
	}

	subcommand := args[0]

	switch subcommand {
	case "build":
		handleBuild(args[1:])
	case "run":
		handleRun(args[1:])
	case "check":
		handleCheck(args[1:])
	case "version", "-v", "--version":
		printVersion()
	case "help", "-h", "--help":
		if len(args) > 1 {
			printSubcommandHelp(args[1])
		} else {
			printGlobalHelp()
		}
	default:
		fail(supportsColor(), "unknown command %q for %q", subcommand, CompilerName)
		fmt.Println()
		printGlobalHelp()
		os.Exit(1)
	}
}

// === Subcommand Handlers ===

// reorderArgs rearranges a subcommand's raw arguments so every flag
// (anything starting with "-") comes before the positional arguments,
// regardless of how the person typed them. Go's stdlib flag package
// stops parsing at the first non-flag token, which would otherwise force
// an unintuitive rule like "the file must come last" (`tinoc build -o out
// prog.tnc` works but `tinoc build prog.tnc -o out` would not); Tinoc's
// CLI accepts either order like most modern tools do.
//
// A flag that takes a value passed as a separate token (`-o out`, not
// `-o=out`) is kept paired with its value by never treating a token
// immediately after a known value-taking flag as positional.
func reorderArgs(args []string) []string {
	valueFlags := map[string]bool{
		"-o": true, "--output": true,
	}

	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			if valueFlags[a] && !strings.Contains(a, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}

func handleBuild(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	var config PipelineConfig

	registerPipelineFlags(fs, &config)

	fs.Usage = func() {
		printSubcommandHelp("build")
	}

	_ = fs.Parse(reorderArgs(args))

	if fs.NArg() < 1 {
		fail(supportsColor(), "missing target file for 'build'")
		fmt.Println("usage: tinoc build <file.tnc> [flags]")
		os.Exit(1)
	}

	config.FilePath = fs.Arg(0)
	if err := readSourceFile(config.FilePath); err != nil {
		fail(supportsColor(), "%v", err)
		os.Exit(1)
	}

	runCompilerPipeline("build", config)
}

func handleRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var config PipelineConfig

	registerPipelineFlags(fs, &config)

	fs.Usage = func() {
		printSubcommandHelp("run")
	}

	_ = fs.Parse(reorderArgs(args))

	if fs.NArg() < 1 {
		fail(supportsColor(), "missing target file for 'run'")
		fmt.Println("usage: tinoc run <file.tnc> [flags]")
		os.Exit(1)
	}

	config.FilePath = fs.Arg(0)
	if err := readSourceFile(config.FilePath); err != nil {
		fail(supportsColor(), "%v", err)
		os.Exit(1)
	}

	runCompilerPipeline("run", config)
}

func handleCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	var config PipelineConfig

	fs.BoolVar(&config.Verbose, "v", false, "enable verbose timing logs")
	fs.BoolVar(&config.Verbose, "verbose", false, "enable verbose timing logs")

	fs.Usage = func() {
		printSubcommandHelp("check")
	}

	_ = fs.Parse(reorderArgs(args))

	if fs.NArg() < 1 {
		fail(supportsColor(), "missing target file for 'check'")
		fmt.Println("usage: tinoc check <file.tnc>")
		os.Exit(1)
	}

	config.FilePath = fs.Arg(0)
	useColor := supportsColor()

	source, err := readSourceFileContent(config.FilePath)
	if err != nil {
		fail(useColor, "%v", err)
		os.Exit(1)
	}

	stage(useColor, "CHECK", "analyzing %s", config.FilePath)

	total, illegal := DumpTokens(source)
	if illegal > 0 {
		fail(useColor, "lexical check failed (%d illegal token(s) of %d total)", illegal, total)
		os.Exit(1)
	}
	ok(useColor, "lexical check passed (%d tokens)", total)

	// Parser is still partial (literals, var/const, basic control flow,
	// and functions), so a parse error here does not necessarily mean the
	// source is invalid Tinoc — only that it uses a construct the parser
	// doesn't cover yet (struct/enum/union bodies, switch, etc). check
	// still surfaces parse errors since they're useful signal either way,
	// but does not fail the command on them.
	_, parseErrs := ParseSource(source)
	if len(parseErrs) > 0 {
		fail(useColor, "syntactic check found %d issue(s) (parser is partial; some may be unsupported syntax, not errors)", len(parseErrs))
		for _, e := range parseErrs {
			fmt.Fprintf(os.Stderr, "  %s\n", e.String())
		}
		os.Exit(1)
	}
	ok(useColor, "syntactic check passed")

	// Sema runs on top of a clean parse. It covers var/const/static
	// var/static const/fn fully; anything else in the program (struct/
	// enum/union/switch/generics/etc) is left unchecked by this pass
	// rather than reported as an error, matching the parser's own
	// "partial, not wrong" stance above.
	_, _, diags := RunSema(config.FilePath, source)

	semaErrs := 0
	for _, d := range diags.All() {
		if d.Severity == SeverityError && d.Stage == "sema" {
			semaErrs++
		}
	}

	if semaErrs > 0 {
		fail(useColor, "semantic check found %s", pluralize(semaErrs, "issue", "issues"))
		for _, d := range diags.All() {
			if d.Stage == "sema" {
				fmt.Fprintln(os.Stderr, "  "+d.Colorize(useColor))
			}
		}
		os.Exit(1)
	}
	ok(useColor, "semantic check passed")
}

// Binds short and long flags to the same option pointers.
func registerPipelineFlags(fs *flag.FlagSet, config *PipelineConfig) {
	fs.StringVar(&config.OutputPath, "o", "", "output path for the binary or generated file")
	fs.StringVar(&config.OutputPath, "output", "", "output path for the binary or generated file")

	fs.BoolVar(&config.Lex, "l", false, "stop at lexer stage and print token stream")
	fs.BoolVar(&config.Lex, "lex", false, "stop at lexer stage and print token stream")

	fs.BoolVar(&config.AST, "a", false, "stop at parser stage and print AST")
	fs.BoolVar(&config.AST, "ast", false, "stop at parser stage and print AST")

	fs.BoolVar(&config.EmitC, "c", false, "stop at codegen stage and output C code")
	fs.BoolVar(&config.EmitC, "emit-c", false, "stop at codegen stage and output C code")

	fs.BoolVar(&config.Verbose, "v", false, "enable verbose compiler log timings")
	fs.BoolVar(&config.Verbose, "verbose", false, "enable verbose compiler log timings")
}

func readSourceFile(path string) error {
	_, err := readSourceFileContent(path)
	return err
}

func readSourceFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}
	return string(data), nil
}

// Compiler Execution Pipeline
//
// Full pipeline: Lexer -> Parser/AST -> Sema -> Codegen -> C compiler ->
// (run mode only) execute the resulting binary. Each stage after the
// lexer can be cut off early via -l/-a/-c for inspecting intermediate
// output; without a cutoff flag, `build` runs through native compilation
// and `run` additionally executes the result.

func runCompilerPipeline(mode string, config PipelineConfig) {
	useColor := supportsColor()

	if config.Verbose {
		stage(useColor, "INFO", "target source: %s", config.FilePath)
		if config.OutputPath != "" {
			stage(useColor, "INFO", "target output: %s", config.OutputPath)
		}
	}

	source, err := readSourceFileContent(config.FilePath)
	if err != nil {
		fail(useColor, "%v", err)
		os.Exit(1)
	}

	// Phase 1: Lexer. Always runs first, since every later phase depends
	// on it. -l/--lex is a cutoff: print the token stream and stop here.

	if config.Lex {
		stage(useColor, "LEXER", "tokenizing %s", config.FilePath)
		total, illegal := DumpTokens(source)
		if illegal > 0 {
			fail(useColor, "%d illegal token(s) found", illegal)
			os.Exit(1)
		}
		ok(useColor, "%d tokens dumped", total)
		return
	}

	stage(useColor, "LEXER", "tokenizing %s", config.FilePath)
	_, illegal := DumpTokensQuiet(source)
	if illegal > 0 {
		fail(useColor, "%d illegal token(s) found", illegal)
		os.Exit(1)
	}
	ok(useColor, "lexing complete")

	// Phase 2: Parser / AST. -a/--ast is a cutoff: print the AST and stop
	// here. Otherwise parse and continue, since sema/codegen need the AST.

	if config.AST {
		stage(useColor, "PARSER", "parsing AST for %s", config.FilePath)
		_, errs := DumpAST(source)
		if errs > 0 {
			fail(useColor, "%d parse error(s) found (parser is partial; see errors above)", errs)
			os.Exit(1)
		}
		ok(useColor, "AST parsed successfully")
		return
	}

	stage(useColor, "PARSER", "parsing %s", config.FilePath)
	program, parseErrs := ParseSource(source)
	if len(parseErrs) > 0 {
		fail(useColor, "%d parse error(s) found (parser is partial; see errors above)", len(parseErrs))
		for _, e := range parseErrs {
			fmt.Fprintf(os.Stderr, "  %s\n", e.String())
		}
		os.Exit(1)
	}
	ok(useColor, "parsed %d top-level statement(s)", len(program.Statements))

	// Phase 3: Sema. Type-checks and resolves names for var/const/static
	// var/static const/fn (see sema.go); Codegen depends on its resolved
	// types, so it always runs before codegen regardless of cutoff flags.

	stage(useColor, "SEMA", "analyzing %s", config.FilePath)
	diags := NewDiagnostics(config.FilePath)
	sema := NewSema(diags)
	sema.Check(program)

	if diags.HasErrors() {
		fail(useColor, "%s found", diags.Summary(useColor))
		diags.PrintStderr()
		os.Exit(1)
	}
	ok(useColor, "semantic analysis passed")

	// Phase 4: Codegen. -c/--emit-c is a cutoff: print the generated C
	// and stop here (or write it to -o if given).

	stage(useColor, "CODEGEN", "transpiling %s to C", config.FilePath)
	gen := NewCodegen(sema, diags)
	gen.sourceDir = filepath.Dir(config.FilePath)
	cCode := gen.Generate(program)

	if diags.HasErrors() {
		fail(useColor, "%s found during codegen", diags.Summary(useColor))
		diags.PrintStderr()
		os.Exit(1)
	}
	ok(useColor, "generated %d line(s) of C", strings.Count(cCode, "\n"))

	if config.EmitC {
		if config.OutputPath != "" {
			if err := os.WriteFile(config.OutputPath, []byte(cCode), 0o644); err != nil {
				fail(useColor, "cannot write %s: %v", config.OutputPath, err)
				os.Exit(1)
			}
			ok(useColor, "wrote %s", config.OutputPath)
		} else {
			fmt.Print(cCode)
		}
		return
	}

	// Phase 5: Native compilation. Discover a C compiler (gcc/clang/cc/
	// tcc, or $CC/$TINOC_CC override), write the generated C plus the
	// embedded tinoc.h runtime header to a work directory, and invoke it.

	cc, err := FindCCompiler()
	if err != nil {
		fail(useColor, "%v", err)
		os.Exit(1)
	}

	outName := determineOutputName(config)

	// `tinoc run` without an explicit -o is throwaway by design: the
	// binary is written inside the scratch work directory and removed as
	// soon as the program finishes, so nothing is left in the user's
	// working directory. `tinoc build` (and `run -o <path>`) keep the
	// binary at the requested location.
	ephemeral := mode == "run" && config.OutputPath == ""

	stage(useColor, "BUILD", "compiling generated C -> %s", outName)
	if useColor {
		fmt.Printf("  %susing%s %s%s%s %s(%s)%s\n", colorDim, colorReset, colorCyan, cc.Name, colorReset, colorDim, cc.Path, colorReset)
	} else {
		fmt.Printf("  using %s (%s)\n", cc.Name, cc.Path)
	}
	if config.Verbose && cc.Version != "" {
		fmt.Printf("  %s\n", cc.Version)
	}

	binPath, workDir, err := compileGeneratedC(cc, config.FilePath, cCode, outName, ephemeral, config.Verbose, useColor)
	if err != nil {
		fail(useColor, "%v", err)
		os.Exit(1)
	}
	ok(useColor, "build succeeded -> %s", binPath)

	if mode == "run" {
		stage(useColor, "EXECUTE", "running %s", binPath)
		fmt.Println()
		exitCode := runBinary(binPath)
		// The scratch dir holds the generated C and (in run mode) the
		// binary itself; drop it now that the program has finished.
		os.RemoveAll(workDir)
		if exitCode != 0 {
			fmt.Println()
			fail(useColor, "program exited with status %d", exitCode)
			os.Exit(exitCode)
		}
		return
	}

	// build mode keeps the binary at binPath; only the scratch C work
	// directory is temporary.
	os.RemoveAll(workDir)
}

// compileGeneratedC writes the generated C source and the embedded
// tinoc.h runtime header into a temporary work directory, then invokes
// the discovered C compiler to produce outName. Returns the path to the
// compiled binary (made absolute so `run` can exec it regardless of the
// working directory) together with the scratch work directory, which the
// caller is responsible for removing once the binary is no longer needed.
//
// When ephemeral is true (`tinoc run` without -o) the binary itself is
// written inside the scratch directory so nothing remains after the
// program finishes; otherwise it is written to outName and kept. On any
// failure the scratch directory is removed here before the error returns.
func compileGeneratedC(cc *CCompiler, sourcePath, cCode, outName string, ephemeral, verbose, useColor bool) (binPath, workDir string, err error) {
	workDir, err = os.MkdirTemp("", "tinoc-build-*")
	if err != nil {
		return "", "", fmt.Errorf("cannot create build work directory: %w", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(workDir)
		}
	}()

	base := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	cFile := filepath.Join(workDir, base+".c")
	if err = os.WriteFile(cFile, []byte(cCode), 0o644); err != nil {
		return "", "", fmt.Errorf("cannot write generated C to %s: %w", cFile, err)
	}
	if err = os.WriteFile(filepath.Join(workDir, "tinoc.h"), []byte(RuntimeHeader), 0o644); err != nil {
		return "", "", fmt.Errorf("cannot write tinoc.h runtime header: %w", err)
	}

	outPath := outName
	if ephemeral {
		// run mode without -o: keep the binary inside the scratch dir so
		// nothing lands in the user's working directory.
		outPath = filepath.Join(workDir, filepath.Base(outName))
	} else if !filepath.IsAbs(outPath) {
		if abs, err := filepath.Abs(outPath); err == nil {
			outPath = abs
		}
	}

	// The source file's directory is added as an include path so
	// #importc'd local headers (`#include "myheader.h"`) resolve even
	// though the generated C is compiled from a temp work directory.
	args := cc.BuildArgs(cFile, outPath, []string{workDir, filepath.Dir(sourcePath)})

	if verbose {
		if useColor {
			fmt.Printf("  %s$ %s %s%s\n", colorDim, cc.Path, strings.Join(args, " "), colorReset)
		} else {
			fmt.Printf("  $ %s %s\n", cc.Path, strings.Join(args, " "))
		}
	}

	cmd := exec.Command(cc.Path, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return "", "", fmt.Errorf("%s failed to compile generated C: %w", cc.Name, err)
	}

	return outPath, workDir, nil
}

// runBinary executes the compiled program, forwarding stdio directly so
// interactive programs behave normally, and returns its exit code (0 on
// success, the child's exit status otherwise, or 1 if the process
// couldn't be started at all).
func runBinary(binPath string) int {
	cmd := exec.Command(binPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "error: cannot run %s: %v\n", binPath, err)
		return 1
	}
	return 0
}

func determineOutputName(config PipelineConfig) string {
	if config.OutputPath != "" {
		return config.OutputPath
	}
	ext := filepath.Ext(config.FilePath)
	return strings.TrimSuffix(filepath.Base(config.FilePath), ext)
}

// Help Screens & Version Info
//

// printVersion prints the compiler name/version plus project metadata
// (repository, creator, website, license) so `tinoc version` doubles as a
// quick reference for where the project lives and who maintains it.
func printVersion() {
	if !supportsColor() {
		fmt.Printf("%s version %s\n", CompilerName, Version)
	} else {
		fmt.Printf("\033[1m%s%s%s version %s%s%s\n", colorCyan, CompilerName, colorReset, colorGreen, Version, colorReset)
	}
	printVersionMeta()
}

// printVersionMeta prints the shared project-metadata block (repository,
// creator GitHub, website, license) used by `tinoc version` and the help
// screens.
func printVersionMeta() {
	fmt.Printf("  repo:     %s\n", RepoURL)
	fmt.Printf("  creator:  %s\n", CreatorGitHub)
	fmt.Printf("  website:  %s\n", Website)
	fmt.Printf("  license:  %s\n", LicenseName)
}

func printGlobalHelp() {
	useColor := supportsColor()

	if !useColor {
		fmt.Print(`TinocLang compiler and transpiler

Usage:
  tinoc <command> [file] [flags]

Commands:
  build       Transpile Tinoc code to C and compile to binary
  run         Transpile, compile, and execute program
  check       Perform lexical, syntactic, and type checks without emitting code
  version     Print compiler version information
  help        Display help info for a command

Global flags:
  -h, --help  Display CLI help information

Run 'tinoc help <command>' for detailed flag usage on specific subcommands.

Links:
  Repository:  https://github.com/tinoc-lang/tinoc
  Creator:     https://github.com/pbarot2009
  Website:     https://tinoc-lang.vercel.app
  License:     Apache-2.0
`)
		return
	}

	bold := "\033[1m"
	fmt.Printf("%sTinocLang%s compiler and transpiler\n\n", bold, colorReset)

	fmt.Printf("%sUsage:%s\n", bold, colorReset)
	fmt.Printf("  tinoc %s<command>%s [file] [flags]\n\n", colorCyan, colorReset)

	fmt.Printf("%sCommands:%s\n", bold, colorReset)
	printCommandLine("build", "Transpile Tinoc code to C and compile to binary")
	printCommandLine("run", "Transpile, compile, and execute program")
	printCommandLine("check", "Perform lexical, syntactic, and type checks without emitting code")
	printCommandLine("version", "Print compiler version information")
	printCommandLine("help", "Display help info for a command")
	fmt.Println()

	fmt.Printf("%sGlobal flags:%s\n", bold, colorReset)
	printFlagLine("-h, --help", "Display CLI help information")
	fmt.Println()

	fmt.Printf("%sRun 'tinoc help <command>' for detailed flag usage on specific subcommands.%s\n\n", colorDim, colorReset)

	fmt.Printf("%sLinks:%s\n", bold, colorReset)
	printCommandLine(RepoURL, "GitHub repository")
	printCommandLine(CreatorGitHub, "Creator's GitHub")
	printCommandLine(Website, "Official website")
	printCommandLine("Apache-2.0", "License")
}

func printCommandLine(name, desc string) {
	fmt.Printf("  %s%-11s%s %s\n", colorCyan, name, colorReset, desc)
}

func printFlagLine(flags, desc string) {
	fmt.Printf("  %s%-13s%s %s\n", colorGreen, flags, colorReset, desc)
}

func printSubcommandHelp(command string) {
	useColor := supportsColor()
	bold := "\033[1m"

	switch command {
	case "build":
		if !useColor {
			fmt.Print(`Usage: tinoc build <file.tnc> [flags]

Transpiles Tinoc source code to C and compiles it using the system C compiler.

Pipeline cutoff flags (testing):
  -l, --lex       Stop after lexer stage and print token stream
  -a, --ast       Stop after parser stage and print AST
  -c, --emit-c    Stop after codegen stage and print transpiled C code

Options:
  -o, --output    Specify output binary or target path
  -v, --verbose   Show detailed compiler execution timing
`)
			return
		}
		fmt.Printf("%sUsage:%s tinoc %sbuild%s <file.tnc> [flags]\n\n", bold, colorReset, colorCyan, colorReset)
		fmt.Println("Transpiles Tinoc source code to C and compiles it using the system C compiler.")
		fmt.Println()
		fmt.Printf("%sPipeline cutoff flags (testing):%s\n", bold, colorReset)
		printFlagLine("-l, --lex", "Stop after lexer stage and print token stream")
		printFlagLine("-a, --ast", "Stop after parser stage and print AST")
		printFlagLine("-c, --emit-c", "Stop after codegen stage and print transpiled C code")
		fmt.Println()
		fmt.Printf("%sOptions:%s\n", bold, colorReset)
		printFlagLine("-o, --output", "Specify output binary or target path")
		printFlagLine("-v, --verbose", "Show detailed compiler execution timing")

	case "run":
		if !useColor {
			fmt.Print(`Usage: tinoc run <file.tnc> [flags]

Transpiles and compiles Tinoc code, then executes the binary immediately.
The binary is written to a temporary directory and removed after execution,
so no build artifacts are left behind. Pass -o to keep the binary.

Options:
  -l, --lex       Stop after lexer stage
  -a, --ast       Stop after parser stage
  -c, --emit-c    Stop after C generation stage
  -v, --verbose   Show detailed execution timing
`)
			return
		}
		fmt.Printf("%sUsage:%s tinoc %srun%s <file.tnc> [flags]\n\n", bold, colorReset, colorCyan, colorReset)
		fmt.Println("Transpiles and compiles Tinoc code, then executes the binary immediately.")
		fmt.Println("The binary is written to a temporary directory and removed after execution,")
		fmt.Println("so no build artifacts are left behind. Pass -o to keep the binary.")
		fmt.Println()
		fmt.Printf("%sOptions:%s\n", bold, colorReset)
		printFlagLine("-l, --lex", "Stop after lexer stage")
		printFlagLine("-a, --ast", "Stop after parser stage")
		printFlagLine("-c, --emit-c", "Stop after C generation stage")
		printFlagLine("-v, --verbose", "Show detailed execution timing")

	case "check":
		if !useColor {
			fmt.Print(`Usage: tinoc check <file.tnc>

Scans and type-checks the Tinoc source file without generating C or binary output.
`)
			return
		}
		fmt.Printf("%sUsage:%s tinoc %scheck%s <file.tnc>\n\n", bold, colorReset, colorCyan, colorReset)
		fmt.Println("Scans and type-checks the Tinoc source file without generating C or binary output.")

	case "version":
		fmt.Printf("%sUsage:%s tinoc %sversion%s\n\n", bold, colorReset, colorCyan, colorReset)
		fmt.Println("Prints the compiler version and project information.")
		fmt.Println()
		printVersion()

	default:
		fail(useColor, "unknown command %q for 'tinoc help'", command)
	}
}

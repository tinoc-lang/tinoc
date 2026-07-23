package src

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	Version      = "0.1.0-dev"
	CompilerName = "tinoc"
)

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

// ----------------------------------------------------------------------
// Subcommand Handlers
// ----------------------------------------------------------------------

func handleBuild(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	var config PipelineConfig

	registerPipelineFlags(fs, &config)

	fs.Usage = func() {
		printSubcommandHelp("build")
	}

	_ = fs.Parse(args)

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

	_ = fs.Parse(args)

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

	_ = fs.Parse(args)

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

	// Cutoff: lexer testing flag.
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

	// Cutoff: parser testing flag.
	if config.AST {
		stage(useColor, "PARSER", "parsing AST for %s", config.FilePath)
		fmt.Println("  (placeholder) AST printing is not yet implemented.")
		return
	}

	// Cutoff: transpiler testing flag.
	if config.EmitC {
		stage(useColor, "CODEGEN", "transpiling %s to C", config.FilePath)
		fmt.Println("  (placeholder) C code generation is not yet implemented.")
		return
	}

	// Full compilation pipeline.
	outName := determineOutputName(config)
	stage(useColor, "BUILD", "transpiling %s -> C -> %s", config.FilePath, outName)
	fmt.Println("  (placeholder) full build pipeline is not yet implemented.")

	if mode == "run" {
		stage(useColor, "EXECUTE", "running ./%s", outName)
		fmt.Println("  (placeholder) execution is not yet implemented.")
	}
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

func printVersion() {
	if !supportsColor() {
		fmt.Printf("%s version %s\n", CompilerName, Version)
		return
	}
	fmt.Printf("\033[1m%s%s%s version %s%s%s\n", colorCyan, CompilerName, colorReset, colorGreen, Version, colorReset)
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

	fmt.Printf("%sRun 'tinoc help <command>' for detailed flag usage on specific subcommands.%s\n", colorDim, colorReset)
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

	default:
		fail(useColor, "unknown command %q for 'tinoc help'", command)
	}
}

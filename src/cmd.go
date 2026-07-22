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
		fmt.Printf("Error: unknown command %q for %q\n\n", subcommand, CompilerName)
		printGlobalHelp()
		os.Exit(1)
	}
}


// Subcommand Handlers

func handleBuild(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	var config PipelineConfig

	registerPipelineFlags(fs, &config)

	fs.Usage = func() {
		printSubcommandHelp("build")
	}

	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Println("Error: missing target file for 'build'.")
		fmt.Println("Usage: tinoc build <file.tn> [flags]")
		os.Exit(1)
	}

	config.FilePath = fs.Arg(0)
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
		fmt.Println("Error: missing target file for 'run'.")
		fmt.Println("Usage: tinoc run <file.tn> [flags]")
		os.Exit(1)
	}

	config.FilePath = fs.Arg(0)
	runCompilerPipeline("run", config)
}

func handleCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	var config PipelineConfig

	fs.BoolVar(&config.Verbose, "v", false, "Enable verbose timing logs")
	fs.BoolVar(&config.Verbose, "verbose", false, "Enable verbose timing logs")

	fs.Usage = func() {
		printSubcommandHelp("check")
	}

	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Println("Error: missing target file for 'check'.")
		fmt.Println("Usage: tinoc check <file.tn>")
		os.Exit(1)
	}

	config.FilePath = fs.Arg(0)
	fmt.Printf("[CHECK] Analyzing %s...\n", config.FilePath)
	fmt.Println(" -> (Placeholder) Lexical, syntax, and type check passed successfully!")
}

// Helper to bind short and long flags to the same option pointers.
func registerPipelineFlags(fs *flag.FlagSet, config *PipelineConfig) {
	fs.StringVar(&config.OutputPath, "o", "", "Output path for the binary or generated file")
	fs.StringVar(&config.OutputPath, "output", "", "Output path for the binary or generated file")

	fs.BoolVar(&config.Lex, "l", false, "Stop at Lexer stage and print token stream")
	fs.BoolVar(&config.Lex, "lex", false, "Stop at Lexer stage and print token stream")

	fs.BoolVar(&config.AST, "a", false, "Stop at Parser stage and print AST")
	fs.BoolVar(&config.AST, "ast", false, "Stop at Parser stage and print AST")

	fs.BoolVar(&config.EmitC, "c", false, "Stop at Codegen stage and output C code")
	fs.BoolVar(&config.EmitC, "emit-c", false, "Stop at Codegen stage and output C code")

	fs.BoolVar(&config.Verbose, "v", false, "Enable verbose compiler log timings")
	fs.BoolVar(&config.Verbose, "verbose", false, "Enable verbose compiler log timings")
}

// @todo
// Compiler Execution Pipeline (Placeholders)

func runCompilerPipeline(mode string, config PipelineConfig) {
	if config.Verbose {
		fmt.Printf("[INFO] Target Source: %s\n", config.FilePath)
		if config.OutputPath != "" {
			fmt.Printf("[INFO] Target Output: %s\n", config.OutputPath)
		}
	}

	// 1. Cutoff: Lexer testing flag
	if config.Lex {
		fmt.Printf("[STAGE: LEXER] Tokenizing %s...\n", config.FilePath)
		fmt.Println(" -> (Placeholder) Token stream dumped successfully.")
		return
	}

	// 2. Cutoff: Parser testing flag
	if config.AST {
		fmt.Printf("[STAGE: PARSER] Parsing AST for %s...\n", config.FilePath)
		fmt.Println(" -> (Placeholder) AST tree printed successfully.")
		return
	}

	// 3. Cutoff: Transpiler testing flag
	if config.EmitC {
		fmt.Printf("[STAGE: CODEGEN] Transpiling %s to C...\n", config.FilePath)
		fmt.Println(" -> (Placeholder) C source emitted successfully.")
		return
	}

	// 4. Full compilation pipeline
	outName := determineOutputName(config)
	fmt.Printf("[STAGE: FULL BUILD] Transpiling %s -> C -> %s...\n", config.FilePath, outName)

	if mode == "run" {
		fmt.Printf("[STAGE: EXECUTE] Running binary ./%s...\n", outName)
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



func printVersion() {
	fmt.Printf("%s version %s\n", CompilerName, Version)
}

func printGlobalHelp() {
	helpText := `TinocLang Compiler & Transpiler

Usage:
  tinoc <command> [file] [flags]

Commands:
  build       Transpile Tinoc code to C and compile to binary
  run         Transpile, compile, and execute program
  check       Perform lexical, syntactic, and type checks without emitting code
  version     Print compiler version information
  help        Display help info for a command

Global Flags:
  -h, --help  Display CLI help information

Run 'tinoc help <command>' for detailed flag usage on specific subcommands.
`
	fmt.Print(helpText)
}

func printSubcommandHelp(command string) {
	switch command {
	case "build":
		fmt.Print(`Usage: tinoc build <file.tn> [flags]

Transpiles Tinoc source code to C and compiles it using the system C compiler.

Pipeline Cutoff Flags (Testing):
  -l, --lex       Stop after Lexer stage and output token stream
  -a, --ast       Stop after Parser stage and output formatted AST
  -c, --emit-c    Stop after Codegen stage and print transpiled C code

Options:
  -o, --output    Specify output binary or target path
  -v, --verbose   Show detailed compiler execution timing
`)
	case "run":
		fmt.Print(`Usage: tinoc run <file.tn> [flags]

Transpiles and compiles Tinoc code, then executes the binary immediately.

Options:
  -l, --lex       Stop after Lexer stage
  -a, --ast       Stop after Parser stage
  -c, --emit-c    Stop after C generation stage
  -v, --verbose   Show detailed execution timing
`)
	case "check":
		fmt.Print(`Usage: tinoc check <file.tn>

Scans and type-checks the Tinoc source file without generating C or binary output.
`)
	default:
		fmt.Printf("Unknown command %q for 'tinoc help'\n", command)
	}
}

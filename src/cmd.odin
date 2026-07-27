package src

import "core:fmt"
import "core:os"
import "core:path/filepath"
import "core:strings"

VERSION       :: "0.1.0-dev"
COMPILER_NAME :: "tinoc"

// Color constants for terminal styling
COLOR_RESET  :: "\x1b[0m"
COLOR_BOLD   :: "\x1b[1m"
COLOR_DIM    :: "\x1b[2m"
COLOR_RED    :: "\x1b[31m"
COLOR_GREEN  :: "\x1b[32m"
COLOR_YELLOW :: "\x1b[33m"
COLOR_CYAN   :: "\x1b[36m"

supports_color :: proc() -> bool {
	term, ok := os.lookup_env("TERM", context.temp_allocator)
	if !ok || term == "dumb" {
		return false
	}
	return true
}

stage :: proc(use_color: bool, label, fmt_str: string, args: ..any) {
	msg := fmt.tprintf(fmt_str, ..args)
	if use_color {
		fmt.printf("%s[%s]%s %s\n", COLOR_CYAN, label, COLOR_RESET, msg)
	} else {
		fmt.printf("[%s] %s\n", label, msg)
	}
}

ok_msg :: proc(use_color: bool, fmt_str: string, args: ..any) {
	msg := fmt.tprintf(fmt_str, ..args)
	if use_color {
		fmt.printf("  %s%s%s\n", COLOR_GREEN, msg, COLOR_RESET)
	} else {
		fmt.printf("  %s\n", msg)
	}
}

fail :: proc(use_color: bool, fmt_str: string, args: ..any) {
	msg := fmt.tprintf(fmt_str, ..args)
	if use_color {
		fmt.eprintf("%serror:%s %s\n", COLOR_RED, COLOR_RESET, msg)
	} else {
		fmt.eprintf("error: %s\n", msg)
	}
}

placeholder :: proc(use_color: bool, fmt_str: string, args: ..any) {
	msg := fmt.tprintf(fmt_str, ..args)
	if use_color {
		fmt.printf("  %s[placeholder]%s %s\n", COLOR_YELLOW, COLOR_RESET, msg)
	} else {
		fmt.printf("  [placeholder] %s\n", msg)
	}
}

// Pipeline_Config holds flags and parameters for a compiler execution run.
Pipeline_Config :: struct {
	file_path:   string,
	output_path: string,
	lex:         bool,
	ast:         bool,
	emit_c:      bool,
	verbose:     bool,
}

// Execute is the main entry point for CLI subcommand processing.
execute :: proc(args: []string) {
	if len(args) < 1 {
		print_global_help()
		os.exit(0)
	}

	subcommand := args[0]
	sub_args := args[1:]

	switch subcommand {
	case "build":
		handle_build(sub_args)
	case "run":
		handle_run(sub_args)
	case "check":
		handle_check(sub_args)
	case "version", "-v", "--version":
		print_version()
	case "help", "-h", "--help":
		if len(sub_args) > 0 {
			print_subcommand_help(sub_args[0])
		} else {
			print_global_help()
		}
	case:
		fail(supports_color(), "unknown command \"%s\" for \"%s\"", subcommand, COMPILER_NAME)
		fmt.println()
		print_global_help()
		os.exit(1)
	}
}

// === Subcommand Handlers ===

handle_build :: proc(args: []string) {
	config: Pipeline_Config

	for arg in args {
		if arg == "-h" || arg == "--help" {
			print_subcommand_help("build")
			os.exit(0)
		}
	}

	positionals := parse_pipeline_flags(args, &config)
	defer delete(positionals)

	if len(positionals) < 1 {
		fail(supports_color(), "missing target file for 'build'")
		fmt.println("usage: tinoc build <file.tnc> [flags]")
		os.exit(1)
	}

	config.file_path = positionals[0]
	if !read_source_file(config.file_path) {
		os.exit(1)
	}

	run_compiler_pipeline("build", config)
}

handle_run :: proc(args: []string) {
	config: Pipeline_Config

	for arg in args {
		if arg == "-h" || arg == "--help" {
			print_subcommand_help("run")
			os.exit(0)
		}
	}

	positionals := parse_pipeline_flags(args, &config)
	defer delete(positionals)

	if len(positionals) < 1 {
		fail(supports_color(), "missing target file for 'run'")
		fmt.println("usage: tinoc run <file.tnc> [flags]")
		os.exit(1)
	}

	config.file_path = positionals[0]
	if !read_source_file(config.file_path) {
		os.exit(1)
	}

	run_compiler_pipeline("run", config)
}

handle_check :: proc(args: []string) {
	config: Pipeline_Config

	for arg in args {
		if arg == "-h" || arg == "--help" {
			print_subcommand_help("check")
			os.exit(0)
		}
	}

	positionals := parse_pipeline_flags(args, &config)
	defer delete(positionals)

	if len(positionals) < 1 {
		fail(supports_color(), "missing target file for 'check'")
		fmt.println("usage: tinoc check <file.tnc>")
		os.exit(1)
	}

	config.file_path = positionals[0]
	use_color := supports_color()

	source, ok := read_source_file_content(config.file_path)
	if !ok {
		os.exit(1)
	}
	defer delete(source)

	stage(use_color, "CHECK", "analyzing %s", config.file_path)

	/*
	// Lexer/Parser checks commented out until implemented in Odin
	total, illegal := dump_tokens(source)
	if illegal > 0 {
		fail(use_color, "lexical check failed (%d illegal token(s) of %d total)", illegal, total)
		os.exit(1)
	}
	ok_msg(use_color, "lexical check passed (%d tokens)", total)

	_, parse_errs := parse_source(source)
	if len(parse_errs) > 0 {
		fail(use_color, "syntactic check found %d issue(s) (parser is partial; some may be unsupported syntax, not errors)", len(parse_errs))
		for e in parse_errs {
			fmt.eprintf("  %s\n", e)
		}
	} else {
		ok_msg(use_color, "syntactic check passed")
	}
	*/

	placeholder(use_color, "lexical and syntactic checks are not yet implemented")
}

// Parses pipeline options and extracts positional argument file paths.
parse_pipeline_flags :: proc(args: []string, config: ^Pipeline_Config) -> [dynamic]string {
	positionals := make([dynamic]string)
	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.has_prefix(arg, "-") {
			switch arg {
			case "-o", "--output":
				if i + 1 < len(args) {
					i += 1
					config.output_path = args[i]
				}
			case "-l", "--lex":
				config.lex = true
			case "-a", "--ast":
				config.ast = true
			case "-c", "--emit-c":
				config.emit_c = true
			case "-v", "--verbose":
				config.verbose = true
			}
		} else {
			append(&positionals, arg)
		}
		i += 1
	}
	return positionals
}

read_source_file :: proc(path: string) -> bool {
	_, ok := read_source_file_content(path)
	return ok
}

read_source_file_content :: proc(path: string) -> (string, bool) {
	data, err := os.read_entire_file(path, context.allocator)
	if err != nil {
		fail(supports_color(), "cannot read %s", path)
		return "", false
	}
	return string(data), true
}

// Compiler Execution Pipeline

run_compiler_pipeline :: proc(mode: string, config: Pipeline_Config) {
	use_color := supports_color()

	if config.verbose {
		stage(use_color, "INFO", "target source: %s", config.file_path)
		if config.output_path != "" {
			stage(use_color, "INFO", "target output: %s", config.output_path)
		}
	}

	source, ok := read_source_file_content(config.file_path)
	if !ok {
		os.exit(1)
	}
	defer delete(source)

	/*
	// Phase 1: Lexer (Commented out until implemented)
	if config.lex {
		stage(use_color, "LEXER", "tokenizing %s", config.file_path)
		total, illegal := dump_tokens(source)
		if illegal > 0 {
			fail(use_color, "%d illegal token(s) found", illegal)
			os.exit(1)
		}
		ok_msg(use_color, "%d tokens dumped", total)
		return
	}

	stage(use_color, "LEXER", "tokenizing %s", config.file_path)
	_, illegal := dump_tokens_quiet(source)
	if illegal > 0 {
		fail(use_color, "%d illegal token(s) found", illegal)
		os.exit(1)
	}
	ok_msg(use_color, "lexing complete")

	// Phase 2: Parser / AST (Commented out until implemented)
	if config.ast {
		stage(use_color, "PARSER", "parsing AST for %s", config.file_path)
		_, errs := dump_ast(source)
		if errs > 0 {
			fail(use_color, "%d parse error(s) found (parser is partial; see errors above)", errs)
			os.exit(1)
		}
		ok_msg(use_color, "AST parsed successfully")
		return
	}

	stage(use_color, "PARSER", "parsing %s", config.file_path)
	program, parse_errs := parse_source(source)
	if len(parse_errs) > 0 {
		fail(use_color, "%d parse error(s) found (parser is partial; see errors above)", len(parse_errs))
		for e in parse_errs {
			fmt.eprintf("  %s\n", e)
		}
		os.exit(1)
	}
	ok_msg(use_color, "parsed %d top-level statement(s)", len(program.statements))
	*/

	// Phase 3: Codegen
	if config.emit_c {
		stage(use_color, "CODEGEN", "transpiling %s to C", config.file_path)
		placeholder(use_color, "C code generation is not yet implemented")
		return
	}

	stage(use_color, "CODEGEN", "transpiling %s to C", config.file_path)
	placeholder(use_color, "C code generation is not yet implemented")

	// Phase 4: Link/compile C binary
	out_name := determine_output_name(config)
	defer delete(out_name)

	stage(use_color, "BUILD", "compiling generated C -> %s", out_name)
	placeholder(use_color, "C compilation/linking is not yet implemented")

	if mode == "run" {
		stage(use_color, "EXECUTE", "running ./%s", out_name)
		placeholder(use_color, "execution is not yet implemented")
	}
}

determine_output_name :: proc(config: Pipeline_Config) -> string {
	if config.output_path != "" {
		return strings.clone(config.output_path)
	}
	ext := filepath.ext(config.file_path)
	base := filepath.base(config.file_path)
	return strings.trim_suffix(base, ext)
}

// Help Screens & Version Info

print_version :: proc() {
	if !supports_color() {
		fmt.printf("%s version %s\n", COMPILER_NAME, VERSION)
		return
	}
	fmt.printf("\x1b[1m%s%s%s version %s%s%s\n", COLOR_CYAN, COMPILER_NAME, COLOR_RESET, COLOR_GREEN, VERSION, COLOR_RESET)
}

print_global_help :: proc() {
	use_color := supports_color()

	if !use_color {
		fmt.print(`TinocLang compiler and transpiler

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

	bold := "\x1b[1m"
	fmt.printf("%sTinocLang%s compiler and transpiler\n\n", bold, COLOR_RESET)

	fmt.printf("%sUsage:%s\n", bold, COLOR_RESET)
	fmt.printf("  tinoc %s<command>%s [file] [flags]\n\n", COLOR_CYAN, COLOR_RESET)

	fmt.printf("%sCommands:%s\n", bold, COLOR_RESET)
	print_command_line("build", "Transpile Tinoc code to C and compile to binary")
	print_command_line("run", "Transpile, compile, and execute program")
	print_command_line("check", "Perform lexical, syntactic, and type checks without emitting code")
	print_command_line("version", "Print compiler version information")
	print_command_line("help", "Display help info for a command")
	fmt.println()

	fmt.printf("%sGlobal flags:%s\n", bold, COLOR_RESET)
	print_flag_line("-h, --help", "Display CLI help information")
	fmt.println()

	fmt.printf("%sRun 'tinoc help <command>' for detailed flag usage on specific subcommands.%s\n", COLOR_DIM, COLOR_RESET)
}

print_command_line :: proc(name, desc: string) {
	fmt.printf("  %s%-11s%s %s\n", COLOR_CYAN, name, COLOR_RESET, desc)
}

print_flag_line :: proc(flags, desc: string) {
	fmt.printf("  %s%-13s%s %s\n", COLOR_GREEN, flags, COLOR_RESET, desc)
}

print_subcommand_help :: proc(command: string) {
	use_color := supports_color()
	bold := "\x1b[1m"

	switch command {
	case "build":
		if !use_color {
			fmt.print(`Usage: tinoc build <file.tnc> [flags]

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
		fmt.printf("%sUsage:%s tinoc %sbuild%s <file.tnc> [flags]\n\n", bold, COLOR_RESET, COLOR_CYAN, COLOR_RESET)
		fmt.println("Transpiles Tinoc source code to C and compiles it using the system C compiler.")
		fmt.println()
		fmt.printf("%sPipeline cutoff flags (testing):%s\n", bold, COLOR_RESET)
		print_flag_line("-l, --lex", "Stop after lexer stage and print token stream")
		print_flag_line("-a, --ast", "Stop after parser stage and print AST")
		print_flag_line("-c, --emit-c", "Stop after codegen stage and print transpiled C code")
		fmt.println()
		fmt.printf("%sOptions:%s\n", bold, COLOR_RESET)
		print_flag_line("-o, --output", "Specify output binary or target path")
		print_flag_line("-v, --verbose", "Show detailed compiler execution timing")

	case "run":
		if !use_color {
			fmt.print(`Usage: tinoc run <file.tnc> [flags]

Transpiles and compiles Tinoc code, then executes the binary immediately.

Options:
  -l, --lex       Stop after lexer stage
  -a, --ast       Stop after parser stage
  -c, --emit-c    Stop after C generation stage
  -v, --verbose   Show detailed execution timing
`)
			return
		}
		fmt.printf("%sUsage:%s tinoc %srun%s <file.tnc> [flags]\n\n", bold, COLOR_RESET, COLOR_CYAN, COLOR_RESET)
		fmt.println("Transpiles and compiles Tinoc code, then executes the binary immediately.")
		fmt.println()
		fmt.printf("%sOptions:%s\n", bold, COLOR_RESET)
		print_flag_line("-l, --lex", "Stop after lexer stage")
		print_flag_line("-a, --ast", "Stop after parser stage")
		print_flag_line("-c, --emit-c", "Stop after C generation stage")
		print_flag_line("-v, --verbose", "Show detailed execution timing")

	case "check":
		if !use_color {
			fmt.print(`Usage: tinoc check <file.tnc>

Scans and type-checks the Tinoc source file without generating C or binary output.
`)
			return
		}
		fmt.printf("%sUsage:%s tinoc %scheck%s <file.tnc>\n\n", bold, COLOR_RESET, COLOR_CYAN, COLOR_RESET)
		fmt.println("Scans and type-checks the Tinoc source file without generating C or binary output.")

	case:
		fail(use_color, "unknown command \"%s\" for 'tinoc help'", command)
	}
}


# Changelog

All notable changes to **Tinoc** are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

_No unreleased changes yet — next release's entries land here._

---

## [0.1.1] - 2026-08-17

Patch release. Structs are now complete end-to-end (generics, methods,
literals, const-mutability, layout diagnostics), generic structs with
methods work across module boundaries, and the project gained a hermetic
Nix flake with matching CI.

### Added

- **Structs, end-to-end**: struct literals (`Point { .x = 1.0, .y = 2.0 }`
  — fields in any order, trailing commas allowed; `Point {}` is an
  explicit zero-initialization; every field must be named and
  type-checked, with exact source spans and `did you mean ...?` hints
  for unknown names). Instance methods take `self ^T` (pointer
  receiver, `self^.x` → `self->x`) or `self T` (by-value receiver);
  static methods are called on the type name itself, including
  monomorphized generic instances (`Rect.square(3.0)`, `Num:i32.of(21)`),
  and calling a static method on a value (or an instance method on the
  type name) is rejected with a hint. Multi-parameter generic structs
  support the chained form `struct Pair:T:U { ... }` alongside
  `struct Map:(K, V) { ... }`; by-value copy/param/return semantics and
  structural size/alignment come from the C11 `typedef struct` emission
  with deterministic dependency ordering. Const bindings are immutable
  all the way down: assigning to a field of a `const` struct — directly
  (`r.w = 1`), through nested structs (`o.inner.v = 1`), or through
  array members (`o.grid[0][1] = 1`) — is rejected with
  `cannot assign to ... (declared const)` and an exact source span, just
  like reassigning the const itself; write-throughs stay legal
  (`self^.x` in mutating methods, `s[i]` on slice parameters). Layout
  diagnostics: circular by-value containment
  (`circular struct layout: A -> B -> A`, with a pointer suggestion) is
  detected at the end of analysis, and a generic-instantiation depth
  limit turns mutually-recursive generic structs into a clear
  `generic instantiation depth limit exceeded` error instead of hanging
  the compiler.
- **Typed array/slice literals**: `[]i32 {}`, `[]f64 { 1.5, 2.5 }`, and
  `[3]i32 { 1, 2, 3 }` spell the element type explicitly — useful in
  struct literal fields and return statements. An empty literal binds
  to a concrete `[0]T` type and codegen emits an empty slice
  (`{ .ptr = NULL, .len = 0 }`), so `Bag { .items = []i32 {} }` works.
- **Samples**: `samples/24_struct_generics.tnc` (generic structs with
  instance + static methods, cross-module-ready `Num:i32.of(21)` style
  calls), `samples/25_struct_literals.tnc` (named-field and zero-init
  literals, typed array-literal fields, slices/optionals inside
  structs, nested/multidimensional arrays in literals),
  `samples/26_struct_value_semantics.tnc` (by-value receivers, static
  constructors, struct copy semantics, slice-field write-through),
  `samples/27_struct_linked_list_methods.tnc` (self-referencing
  pointer structs with methods, node arenas, pointer field writes), and
  `samples/28_struct_module_generics.tnc` plus
  `samples/modules/containers.tnc` (generic structs with instance +
  static methods instantiated across module boundaries).
- **Nix support**: `flake.nix` adds `packages.default` (a hermetic
  `buildGoModule` build with the same `-trimpath` and version ldflags
  `build.sh` injects, installing the C11 runtime header into
  `$out/include` and `$out/share/tinoc`), `devShells.default` (pinned
  Go toolchain matching `go.mod`, golangci-lint, gopls, gcc + clang,
  gdb/valgrind on Linux or lldb on macOS), and `checks.default` so
  `nix flake check` runs gofmt, go vet, `go test -race`, and the full
  end-to-end samples suite — across x86_64/aarch64 Linux and Darwin.
  `.envrc` loads the shell via nix-direnv, and CI gained a `nix` job
  on Linux + macOS using Determinate Nix and Magic Nix Cache
  (`nix flake check`, `nix build .#tinoc`, golangci-lint / go vet /
  race tests inside the dev shell).
- **Smart module system**: `#import` now resolves user modules semantically.
  Every form is supported — namespace (`#import math;`), wildcard
  (`#import math.*;`), single symbol (`#import math.PI;`), selected symbols
  with per-symbol aliases (`#import math.{a, b as c};`), namespace aliases
  (`#import math as m;`), submodule paths (`#import shapes.vec;`), and
  quoted file imports (`#import "lib/tools.tnc" as t;`). Paths resolve
  relative to the importing file; `a.b.c` finds `a/b/c.tnc`, then the
  directory forms `a/b/c/mod.tnc` / `a/b/c/c.tnc` — **directories become
  modules automatically** (`shapes/mod.tnc` is the module `shapes`).
  Imports load recursively with cycle detection and cache by absolute
  path, so diamond imports share one module instance; re-importing the
  same module is idempotent. Module-name collisions and `std.*` imports
  (not available yet) get clear diagnostics.
- **`module` keyword**: `module name;` names a file's module (files
  without one take their file stem); `module name { ... }` groups
  declarations into an in-file namespace, with nesting
  (`module a { module b { ... } }` → `a.b.member`) and dotted names
  (`module a.b { ... }`). `pub` marks exports; private items are rejected
  when imported (`symbol x is private to module math`).
- **Generics end-to-end**: `fn name:T(...)`, `struct Name:T { ... }`, and
  `alias Name:T = ...;` (including multi-param `:(K, V)` forms) are
  monomorphized — each concrete type-argument set produces one copy with
  a mangled C name (`Pair:i32` → `tnc_Pair_i32`), cached per instance.
  Calls support explicit type arguments (`identity:str(x)`) and inference
  from argument types (`identity(x)`), and generics work across module
  boundaries (`shapes.Circle:f64`, `math.identity:i32(42)`).
- **Generics importable by name**: pub generic fn/struct/alias templates
  bind through every import form like plain symbols — `#import
  math.identity;` (bare `identity:i32(42)` / inferred `identity(7)`),
  `#import math.identity as id;`, `#import math.{Pair};` (bare
  `Pair:f64 { ... }`), `#import box.Opt;`, and `#import math.*;` binds
  every pub generic bare. A bare instantiation and the qualified call
  share one mangled C instance (`identity:i32(42)` and
  `math.identity:i32(42)` emit a single `tnc_math_identity_i32`); private
  generics report `symbol x is private to module math` on both paths;
  module-block generics are reachable through dotted chains
  (`math.physics.blockid:i32(42)`).
- **Generic bodies compose**: a generic fn/method body may reference the
  defining module's own generics (`makePair:(K, V)` returning
  `Pair:(K, V) { ... }`) — instantiated bodies have their type
  expressions substituted too, and are checked against the defining
  module's analyzer, so module-private helpers and consts resolve
  exactly as in the defining file.
- **Single merged C output**: every loaded module compiles into one C
  translation unit in load order (imports before importers), with type
  typedefs, prototypes, and file-scope data hoisted so call order across
  modules never matters; module items get mangled C names
  (`math.abs` → `tnc_math_abs`) so same-named items never collide.
- **Sample module demo**: `samples/modules/` — a multi-file demo
  (`main.tnc`, `math.tnc`, `shapes/mod.tnc`, `shapes/vec.tnc`) exercising
  every import form, the `module` keyword (declaration + block),
  directory modules, and generics across modules; `samples/build.sh` runs
  it alongside the single-file samples.
- **Docs**: `syntax.md`'s Modules and Preprocessor sections now document
  the full `#import` grammar, the `module` keyword, `pub`/private
  visibility, directories-as-modules, and module resolution rules, plus
  a new C Interop section covering `#importc` (previously undocumented)
  and `extern "C" fn`; `README.md`'s language-support table reflects the
  module system and generics.
- **Optionals**: `?T` — a `{ T value; bool has_value; }` wrapper emitted
  as a named typedef (`?i32` -> `tnc_opt_i32`), with `null` as the empty
  value, `x orelse fallback` defaulting (the fallback is only evaluated
  when the optional is null), `x?` payload unwrap, and `== null` /
  `!= null` presence checks. A plain `T` value (or `null`) auto-wraps
  into a `?T` wherever one is expected — initializers, call arguments,
  return values, assignments — and untyped literals adapt to the payload
  type (`var x ?f64 = 5;`). Comparing an optional with a non-null value,
  `orelse` on a non-optional, `?` on a non-optional, and optionals of
  arrays or `void` are all rejected with clear diagnostics. Sample:
  `samples/23_optionals.tnc`.
- **Arrays**: `[N]T`, `[_]T` (size inferred from the literal), and
  `[N:x]T` sentinel-terminated arrays — array literals with inferred or
  explicit element types, index access and element assignment
  (`arr[i]`, `arr[i] = v`), the `.len` property (compile-time constant
  for arrays), and `for arr |x|` collection iteration. Multidimensional
  row-major arrays (`[4][5]f32`) support chained indexing (`mat[i][j]`)
  and row-by-row iteration; iterating a multidimensional array directly
  is rejected with a hint.
- **Slices**: `[]T` — a fat pointer `{ ptr: ^T, len: usize }` that
  views an array's storage. Arrays convert implicitly to slices at call
  sites and in assignments, so functions take collections as slices
  (`fn total(s []i32)`), index and mutate through them (write-through
  to the backing array), read `.len` at runtime, and iterate with
  `for s |v|`. Array parameters, array returns, and whole-array
  assignment are rejected with a "use a slice" diagnostic; slice types
  are emitted as named typedefs (`tnc_slice_i32`) so prototypes and
  definitions share one C type.
- **Samples**: `samples/19_array_basics.tnc` through
  `samples/22_array_functions.tnc` covering array literals/types,
  slices and implicit conversion, multidimensional + sentinel arrays,
  and functions over collections.
- **Docs**: `syntax.md`'s Array section (previously a placeholder) now
  documents arrays, slices, multidimensional arrays, and
  sentinel-terminated arrays with working examples.
- **`tinoc version` / `tinoc help`**: the CLI now reports project
  metadata — the GitHub repository (`tinoc-lang/tinoc`), the creator's
  GitHub (`pbarot2009`), the official website, and the license — in
  `tinoc version`, the `Links` section of `tinoc help`, and the new
  `tinoc help version` screen.

- **Enums**: `enum Name { Variant, Other(type), ... }` — fieldless
  variants compile to plain C enums; variants with payloads become
  tagged unions (tag + per-variant anonymous struct), so multi-field
  payloads never overlap. Enums support instance methods (`self
  ^Name`), static methods, equality, and being passed/returned like any
  other type.
- **Enum `switch`**: exhaustive enum switches (all variants listed, no
  `_` arm needed) with pattern binding — `Shape.Rect(w, h) => { ... }`
  binds the payload fields directly; `_` wildcards discard a payload
  slot. Missing-return analysis understands exhaustive enum switches.
- **Unions**: `union Name { field type; ... }` with C-style shared-memory
  fields — writing `as_int` and reading `as_float` reinterprets the same
  bytes (IEEE-754 punning works end-to-end). Unions reuse struct field
  syntax, support instance methods (`self ^Name`) and static methods,
  and reject comparisons/arithmetic with clear diagnostics. Generic
  unions (`union Pair:T`) are rejected with a "not yet supported"
  diagnostic, matching structs/enums.
- **Samples**: `samples/14_enum_basics.tnc` through
  `samples/17_str_strings.tnc` covering fieldless enums, tagged unions
  with pattern matching, enum methods, and `str` semantics, plus
  `samples/18_combo.tnc`, which exercises structs, enums (pattern
  matching), unions (type punning), `str`, `switch`, and methods all in
  one program.

- **Installers**: `install.sh` / `install.ps1` fetch the latest GitHub release,
  verify it against the release `SHA256SUMS` manifest, and extract it into
  `~/.tinoc/` — with `VERSION` tracking, update checks that ask before
  upgrading, `--local` source builds, `--check`, `--uninstall`, and
  `--version` / `--force` / `--dir` flags.
- **CI workflow** (`.github/workflows/ci.yml`) runs on every push and pull
  request: deps, fmt-check, vet, lint, race tests, and a release build
  (`build.sh ci` / `build.ps1 ci`) across ubuntu/macos/windows, plus an
  installer smoke test in `--local` mode.
- The release workflow now attaches a `SHA256SUMS` manifest to every release,
  so installers can verify downloads.
- **`.gitattributes`**: Go and shell sources are pinned to LF line endings on
  checkout everywhere, so `gofmt -l` no longer false-fails on Windows CI
  (where git used to check out CRLF).

### Fixed

- **Cross-module generic structs with methods**: instantiating a
  module's generic struct that declared methods (`struct Box:T {
  fn get(self ^Box:T) ... }` imported as `containers.Box:i32`) failed
  semantic analysis — the method signature/body substitution rewrote
  the type arguments but kept the template's bare base (`self
  ^Pair:(K, V)` became `^Pair:(i32, str)`), which the importer could
  not resolve because the template lives under its module-qualified
  key. `substituteTypeExpr` now substitutes a generic's base too when
  it names the template itself, so cross-module instances resolve to
  the concrete canonical type (`containers.Pair:(i32, str)`) and their
  methods check and codegen correctly.
- **`#importc` local headers next to modules in subdirectories**: a
  module file importing its own local header (`#importc "vecmath.h"`
  inside `lib/mathc.tnc` with `vecmath.h` beside it) emitted the right
  quoted include into the merged C, but the C compile step only put the
  *entry file's* directory on the include path — so the header was not
  found and the build failed. `compileGeneratedC` now receives every
  loaded module's directory as an `-I` path (in load order,
  deduplicated), so local headers resolve wherever they live.
- **Generic struct instantiation recursion**: a generic method whose
  signature references the struct's own type (`fn sum(self ^Pair:T)`)
  re-entered instantiation while the instance was still being built and
  recursed forever (clone → register method → resolve signature →
  re-instantiate). Instances are now cached before their method
  signatures resolve, so the re-entrant call hits the cache.
- **Generic bodies kept stale type parameters**: after monomorphization a
  generic body's type expressions still carried the template's type
  parameters (`return Pair:(K, V) { ... }` inside a `makePair:(K, V)`
  instance), so re-checking the body failed on K/V. `substituteBodyTypes`
  now rewrites every reachable type expression in the cloned body from
  the same substitution environment as the signature.
- **Cross-module generic instances checked against the caller**: method
  and fn bodies of a generic instantiated from another module were
  checked in the caller's scope, so module-private names referenced by
  the body did not resolve. Instances now carry the defining module's
  analyzer, and the caller's concrete type arguments are mirrored into
  it, so bodies resolve module-local helpers/consts and caller-provided
  types alike.
- **Stale `#importc` parse cache**: the disk cache keyed only on the
  wrapper text and dumper identity, so two `#importc "vecmath.h"` from
  different directories (or an edited local header) could be served the
  stale parse of a different file under the same name — silently missing
  or wrong declarations. The key now folds in each local header's
  resolved absolute path plus a hash of its current contents, so
  same-named headers across directories and edits always re-parse.
- **Module-file blocks unreachable from importers**: a
  `module name { ... }` block inside a module file (`module math;` …
  `module physics { pub const g; }`) could not be reached cross-module
  — `math.physics.g` failed with "module math has no public member
  physics". Blocks now publish a pub-only projection into the file
  module's namespace, so dotted chains (`math.physics.g`,
  `math.physics.weight`) resolve, nested blocks chain (`math.a.b.VAL`),
  and private block members stay file-private with a clear diagnostic.
- **Private `const`/`var` diagnostics**: importing a private top-level
  `const`/`var` reported "module x has no public symbol y"; it now
  reports "symbol y is private to module x", matching private
  functions. Private globals are tracked per module and checked in both
  the symbol-import and qualified-access paths.
- **Module `str` globals**: top-level `const`/`var` of type `str` (or
  arrays/slices of `str`) in module files emitted invalid C
  (`static const str x = tinoc_str_lit(...)` — a function call in a
  static-storage initializer). Literals now emit a constant brace
  initializer (`{ .data = "...", .len = N }`); other str initializers
  fall back to external linkage. This broke wildcard-imported string
  constants.
- **Nested module blocks**: `module a { module b { pub const VAL; } }`
  failed to resolve `a.b.VAL` — block views registered only under their
  local segment, so dotted chains through nested/dotted block namespaces
  now register every dotted prefix of the full name.
- **Dead module globals**: unreferenced private top-level `const`/`var`
  in module files are no longer emitted into the merged C output (they
  were always emitted as `static`, making the C compiler warn with
  `-Wunused-const-variable`/`-Wunused-variable` and padding the output
  with dead data). A global is kept when it is `pub` (reachable through
  its module namespace, e.g. `math.PI`), a `module name { ... }` block
  member, or referenced by a bare identifier anywhere in the
  compilation.

- **CI / C-interop tests**: the `#importc` error-checking tests
  (`TestCImport_UndefinedMember`, `TestCImport_WrongArgCount`,
  `TestCImport_WrongArgType`, `TestCImport_DuplicateAlias`) now skip when no
  clang or gcc is available to parse C headers, matching the rest of the
  suite (`requireCC`, `TestCImport_ExternVars`). Previously they hard-failed
  on runners without a C toolchain — e.g. the Windows CI runner — which
  broke `build.sh ci` / `build.ps1 ci` on both push and pull-request events.

- **`tinoc run` no longer leaves binaries behind**: the compiled binary is
  written to a temporary work directory and removed as soon as the
  program finishes, so `tinoc run` leaves no artifacts in the working
  directory (pass `-o <path>` to keep one). The generated-C scratch
  directory is also cleaned up after both `build` and `run` — it
  previously leaked in the system temp dir on every invocation.

- **Float literals**: underscore digit separators (e.g. `1_000.5`) are now
  stripped before emission into C, so float literals with separators no
  longer generate C that fails to compile.
- **For-collection loops**: the loop index is compared against the
  collection length as `size_t`, eliminating a `-Wsign-compare` warning
  when iterating slices (and the generated loop now binds the collection
  once, evaluating it exactly once).
- **Slice `.len`**: typed as `i32` so `i < s.len` type-checks; codegen
  casts the C `size_t` field so user comparisons do not trigger
  `-Wsign-compare`.
- **Const arrays**: iterating a `const` array no longer emits a
  `-Wdiscarded-qualifiers` warning (the loop uses a const pointer).

- **Compiler**: calling an instance method that was declared without a
  `self` parameter (e.g. `fn make() Data { ... }` inside a struct/union
  body, invoked as `d.make()`) previously panicked with an
  index-out-of-range error; it now reports a proper "needs a self
  parameter" diagnostic.
- **str**: `<`, `>`, `<=`, `>=` on `str` are now rejected with a clear
  diagnostic (only `==`/`!=` are defined, via content comparison) instead
  of emitting invalid C. Switching on a `str` is likewise a proper
  semantic error. `str ==`/`!=` compare by content through the
  `tinoc_str_eq` runtime helper.
- **Installers**: `--local` no longer prompts to "update" when the built
  version equals the installed version — the fresh source build is installed
  directly. Re-running the installer with the latest version already
  installed now exits before downloading anything.
- **Installers**: `--force`/`--yes` now actually reinstalls over an
  identical installed version instead of reporting "already installed".
- **CLI**: color output is only emitted when stdout is a real terminal, so
  piped commands like `tinoc version | awk ...` return plain text (this was
  breaking the installer's version parsing on systems with TERM set).
- **Version reporting**: `build.sh` / `build.ps1` now inject the version into
  `src.Version` via `-ldflags` (the old `-X main.version` target never
  existed, so release binaries always reported the hardcoded `0.1.0`).
- **Installers**: installed-version comparisons normalize a leading `v`, so a
  `VERSION` file written as `v0.1.0` (e.g. by an earlier installer) is
  correctly treated as equal to `0.1.0` instead of triggering a spurious
  "update available" prompt.
- **Installers**: `--verbose` now prints the download/fetch commands it runs.
- **Installers**: confirmation prompts now decline gracefully when there is no
  interactive terminal (CI, piped input, cron) instead of aborting the whole
  install with an error — the binary still installs and manual PATH
  instructions are printed.
- **Installers**: the update prompt now appears *before* the download starts,
  so a declined update never wastes bandwidth on a full release download.
- **install.ps1** now detects the real OS and architecture (linux/darwin/
  windows × amd64/arm64) and installs the matching binary name, making it
  fully cross-platform under PowerShell 7+ — matching `install.sh`. It also
  prefers the repo's `build.sh` when running `-Local` on macOS/Linux.
- **build.ps1**: OS/architecture detection now matches `build.sh`, so
  `install.ps1 -Local` on macOS/Linux cross-targets correctly.
- **install.sh**: archive binary discovery no longer relies on the GNU-only
  `find -maxdepth`, which errored on macOS's BSD find; it also offers to
  create a missing shell rc file when adding `~/.tinoc/bin` to PATH.

## [0.1.0] - 2026-08-05

First public release. Tinoc transpiles to **C11** and ships a CLI with
`build`, `run`, and `check` subcommands (`-l/--lex`, `-a/--ast`,
`-c/--emit-c` pipeline cutoffs).

### Added

- **Core declarations**: `var`, `const`, `static var`, `static const` with
  explicit or inferred types, const-mutability enforcement, and
  redeclaration/shadowing checks.
- **Functions**: `fn name(params...) Ret { ... }` with forward calls,
  duplicate-parameter detection, argument count/type checking, return-type
  agreement, and missing-return analysis.
- **Structs**: `struct Name { ... }` with typed fields, struct literals
  (`Point { .x = 1.0, .y = 2.0 }`), field access, instance methods
  (`fn m(self ^Name, ...)` mutating through `self^.field`), static methods,
  struct-typed params/returns, and by-value copy semantics. Self-referencing
  structs (`struct Node { next ^Node; }`) work via pointers.
- **Pointers**: `^T` types, `&x` address-of, `x^` dereference, pointer
  params/returns, `null` comparison.
- **Control flow**: `if` / `else if` / `else`, `while`, range `for`
  (`for 0..10 |i| { ... }`), `break`, `continue`.
- **Literals & operators**: integers (decimal/hex/octal/binary with `_`
  separators), floats, strings, chars, bools, `null`; arithmetic,
  comparison, logical (`and`/`or`), and bitwise operators; wrapping
  arithmetic (`+%`, `-%`, `*%`).
- **C interop**:
  - `#importc "header.h" [as alias];` — parses real C headers via clang's
    JSON AST (or gcc's `-aux-info` fallback) for type-safe calls and
    constants.
  - `extern "C" fn name(.symbol)?(...) Ret;` — hand-declared C functions
    with automatic `str` → `char*` argument unwrapping.
- **Runtime header** (`tinoc.h`): C11 type aliases (`u8`..`i128`, `f32`/`f64`,
  `str`, `char32`), slice/optional/error-union helpers, and `_Generic`
  saturating arithmetic dispatch.
- **Samples**: `samples/00_*.tnc` through `samples/13_*.tnc` covering the
  supported feature set (including struct basics, methods, nested/pointer
  structs, and a scoreboard example).
- **Tooling**: `build.sh` / `build.ps1` (build, test, vet, fmt, lint, install,
  cross-compile), embedded `tinoc.h`, and a GitHub Actions release workflow
  that publishes binaries as `.tar.gz` / `.zip` archives on tag push.

### Changed

- Transpile target moved to **C11** (was C99) to enable `_Generic`-based
  runtime features and future type-system growth.
- Postfix `^` dereference now binds at postfix precedence, so expressions
  like `a * self^.x` parse correctly.

### Fixed

- Comparison operators now adapt untyped literals (`f32Val > 0.0`), matching
  the arithmetic branch.

### Security

- No known security issues.

[Unreleased]: https://github.com/tinoc-lang/tinoc/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/tinoc-lang/tinoc/releases/tag/v0.1.1
[0.1.0]: https://github.com/tinoc-lang/tinoc/releases/tag/v0.1.0

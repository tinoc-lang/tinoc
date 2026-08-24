# tree-sitter-tinoc

A complete [Tree-sitter](https://tree-sitter.github.io/) grammar for
**TinocLang** (`.tnc` files), generated from the official `syntax.md`
specification.

## Features

- Full syntax coverage: modules, `#import` / `#importc` / `#run` / `#partial`
  directives, functions, structs with methods (`self ^T`) and `static fn`,
  enums with payloads, unions, generics (`Name:T`, `:(K, V)`), error unions
  (`!E` prefix and `T1!T2` infix return types), optional types (`?T`),
  pointers (`^T`), slices/arrays, ranges (`for 0..10 |n| { ... }` captures),
  switch pattern matching, `try`/`catch |err|`, all documented operators
  (wrapping/saturating arithmetic, `<<|`, `||`, `**`, `++`, compound assigns),
  C-style extern declarations, line & block comments.
- Editor queries included:
  - `queries/highlights.scm` — full syntax highlighting
  - `queries/locals.scm` — locals/references (goto-definition groundwork)
  - `queries/tags.scm` — symbol tags (outline / document symbols)
  - `queries/injections.scm` — language injections

## Development

```sh
npm install            # installs tree-sitter-cli
npm run generate       # regenerate parser from grammar.js
npm test               # run corpus tests in test/corpus/
npx tree-sitter parse examples/comprehensive.tnc   # parse a sample file
```

## Building the parser library

```sh
npx tree-sitter build -o tinoc.so
```

## Setting up in Helix

### Option A — prebuilt shared library (recommended)

1. **Build the grammar:**

   ```sh
   cd tree-sitter-tinoc
   npx tree-sitter generate
   npx tree-sitter build -o tinoc.so
   ```

2. **Install into your Helix runtime.** Replace `~/.config/helix` with your
   actual config dir (`%appdata%\helix` on Windows):

   ```sh
   mkdir -p ~/.config/helix/runtime/grammars
   mkdir -p ~/.config/helix/runtime/queries/tinoc
   cp tinoc.so ~/.config/helix/runtime/grammars/tinoc.so
   cp queries/*.scm ~/.config/helix/runtime/queries/tinoc/
   ```

3. **Register the language.** Add to `~/.config/helix/languages.toml`:

   ```toml
   [[language]]
   name = "tinoc"
   scope = "source.tinoc"
   file-types = ["tnc"]
   roots = ["tinoc.json", ".git"]
   comment-token = "//"
   block-comment-tokens = { start = "/*", end = "*/" }
   indent = { tab-width = 4, unit = "    " }
   formatter = { command = "tinoc", args = ["fmt"] }   # optional
   auto-format = false
   ```

4. **Restart Helix** and verify:

   ```sh
   hx --health tinoc
   # should show: Highlighters: tree-sitter, ... Grammar: tinoc ✓
   hx main.tnc
   ```

### Option B — let Helix clone & build it

If this repository is reachable via git, point Helix at it:

```toml
# ~/.config/helix/languages.toml
[[language]]
name = "tinoc"
scope = "source.tinoc"
file-types = ["tnc"]
roots = ["tinoc.json", ".git"]
comment-token = "//"
indent = { tab-width = 4, unit = "    " }

[[grammar]]
name = "tinoc"
source = { path = "/absolute/path/to/tree-sitter-tinoc", rev = "main" }
```

Then:

```sh
hx --grammar fetch    # clones the repo into runtime/grammars/sources/
hx --grammar build    # builds tinoc.so
```

> Note: Option B requires the directory to be a git repository.

## Notes for LSP work later

The grammar node names are stable and documented in `src/node-types.json`
(generated). Key nodes for an LSP: `function_declaration`,
`struct_declaration`, `enum_declaration`, `variable_declaration`,
`const_declaration`, `parameter`, `generic_type`, `import_declaration`.
The `locals.scm` query already tags definitions/references, which most
tree-sitter-based jump-to-definition implementations consume directly.

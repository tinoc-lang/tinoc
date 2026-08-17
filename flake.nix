# Tinoc — Nix flake
#
# Hermetic, zero-drift development and CI support for the Tinoc compiler:
#
#   packages.default (tinoc)  — the compiler, built with buildGoModule,
#                               `-trimpath` and the same version ldflags
#                               build.sh injects (`src.Version`), with the
#                               C11 runtime header (tinoc.h) installed into
#                               $out/share/tinoc and $out/include.
#   devShells.default         — Go (pinned to the go.mod toolchain), the C11
#                               toolchain used to compile generated code
#                               (gcc + clang on Linux, clang on macOS),
#                               golangci-lint, gofmt, gdb, and valgrind.
#   checks.default            — the `nix flake check` derivation: gofmt,
#                               go vet, `go test -race ./...`, and the full
#                               end-to-end samples suite (generated C is
#                               compiled and executed with the C compiler).
#
# Systems: x86_64-linux, aarch64-linux, x86_64-darwin, aarch64-darwin.
#
# NOTE: the nixpkgs input is pinned to a full revision so the flake is
# reproducible without a lock file; run `nix flake lock` once to write and
# commit flake.lock for the record.

{
  description = "Tinoc — This Is Not C — a modern systems programming language that transpiles to C11";

  inputs = {
    # Pinned nixpkgs-unstable (2026-08-14): ships go 1.26.5, which matches
    # go.mod exactly. Bump by running `nix flake update nixpkgs`.
    nixpkgs.url = "github:NixOS/nixpkgs/8be7bd0c83f12e2e3bbba07c9044d6fed9e66f7f";
  };

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      # Apply f to every supported system with its nixpkgs instantiation.
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f (import nixpkgs { inherit system; }));

      # Version injected into src.Version via -ldflags, mirroring build.sh's
      # `git describe` fallback chain: the flake's git revision when
      # available, else "dev".
      version = self.shortRev or "dev";
    in
    {
      packages = forAllSystems (
        pkgs:
        let
          go = pkgs.go_1_26; # go.mod: `go 1.26.5`
        in
        {
          default = pkgs.buildGoModule {
            pname = "tinoc";
            inherit version;
            src = self;

            inherit go;
            # go.mod has zero external dependencies, so there is no vendor
            # hash to pin.
            vendorHash = null;

            # Mirror build.sh: `-trimpath` + `-X
            # github.com/tinoc-lang/tinoc/src.Version=<ver>` (release mode
            # also strips symbols/debug info with -s -w).
            buildFlags = [ "-trimpath" ];
            ldflags = [
              "-s"
              "-w"
              "-X github.com/tinoc-lang/tinoc/src.Version=${version}"
            ];

            # Install the C11 runtime header so generated C can include it:
            # $out/include/tinoc.h for C consumers, $out/share/tinoc/tinoc.h
            # as a data file (mirrors how build/run embed it into the binary).
            postInstall = ''
              install -Dm644 src/runtime/tinoc.h "$out/include/tinoc.h"
              install -Dm644 src/runtime/tinoc.h "$out/share/tinoc/tinoc.h"
            '';

            meta = {
              description = "This Is Not C — a modern systems programming language that transpiles to C11";
              homepage = "https://github.com/tinoc-lang/tinoc";
              license = pkgs.lib.licenses.asl20;
              mainProgram = "tinoc";
              platforms = systems;
            };
          };

          # Alias so both `nix build .#tinoc` and `nix build` work.
          tinoc = self.packages.${pkgs.system}.default;
        }
      );

      devShells = forAllSystems (
        pkgs:
        let
          # C11 compilers: gcc + clang on Linux; clang (the system cc) on
          # macOS, where gcc does not build. Debuggers: gdb/valgrind are
          # Linux-only in nixpkgs; macOS uses lldb instead.
          ccTools = with pkgs; [
            (if pkgs.stdenv.isLinux then gcc else clang)
            clang
          ];
          debugTools =
            if pkgs.stdenv.isLinux then
              with pkgs; [
                gdb
                valgrind
              ]
            else
              with pkgs; [ lldb ];
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go_1_26
              golangci-lint
              # gofmt ships with Go; keep it listed for clarity.
              gopls
              # The flake-built compiler, available as `tinoc`.
              self.packages.${pkgs.system}.default
            ] ++ ccTools ++ debugTools;

            shellHook = ''
              echo "Tinoc dev shell — $(go version | awk '{print $3}')"
              echo "  build:  nix build .#tinoc"
              echo "  check:  nix flake check"
              echo "  test:   go test -race ./..."
              echo "  samples: ./samples/build.sh run"
            '';
          };
        }
      );

      # `nix flake check` runs this derivation: the same gates the project's
      # own CI pipeline enforces (fmt-check, vet, race tests, end-to-end
      # samples), so a flake check is a full confidence gate.
      checks = forAllSystems (
        pkgs:
        let
          go = pkgs.go_1_26;
        in
        {
          default = pkgs.stdenv.mkDerivation {
            pname = "tinoc-checks";
            inherit version;
            src = self;

            nativeBuildInputs = [
              go
              pkgs.golangci-lint
              pkgs.stdenv.cc # gcc on Linux, clang on macOS
            ];

            # The generated-C build steps write to $TMPDIR and need a Go
            # build cache; HOME may be unwritable in the sandbox.
            env = {
              HOME = "/tmp";
              GOCACHE = "/tmp/go-cache";
              GOPATH = "/tmp/go-path";
            };

            buildPhase = ''
              runHook preBuild

              echo "==> gofmt check"
              test -z "$(gofmt -l .)" || {
                echo "gofmt needed on:"; gofmt -l .; exit 1;
              }

              echo "==> go vet"
              go vet ./...

              echo "==> go test -race"
              go test -race -timeout 600s ./...

              echo "==> build tinoc"
              go build -trimpath -ldflags "-X github.com/tinoc-lang/tinoc/src.Version=${version}" -o tinoc .

              echo "==> end-to-end samples (generated C compiled and run)"
              ./samples/build.sh run

              runHook postBuild
            '';

            installPhase = ''
              mkdir -p "$out"
              echo "tinoc checks passed" > "$out/checks-passed"
            '';
          };
        }
      );
    };
}

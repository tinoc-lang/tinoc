# Release Checklist

This checklist documents everything that must happen — and everything that
should be verified — before, during, and after shipping a Tinoc release.

## v0.1.0 — release checklist

### 1. Code health

- [ ] `./build.sh fmt-check` passes (all files gofmt-clean).
- [ ] `./build.sh vet` passes (`go vet ./...`).
- [ ] `./build.sh test` passes (`go test ./...`, full suite including
      end-to-end codegen tests that compile and run generated C11).
- [ ] `./build.sh build` produces `build/tinoc` successfully.
- [ ] Spot-check samples end-to-end with the freshly built binary:
      `./build/tinoc run samples/10_struct_basics.tnc` (and friends).

### 2. Release prep

- [ ] `src/cmd.go` `Version` default matches the release version (`0.1.0`);
      release binaries get it overridden from the tag by `build.sh`'s
      ldflags, so `./build/tinoc version` reports the actual release.
- [ ] `CHANGELOG.md` has an entry for the version (move `[Unreleased]`
      items into a dated `[x.y.z]` section and reset `[Unreleased]`).
- [ ] `README.md` language-support table is accurate for the release.
- [ ] `CHECKLIST.md` matches the release's own process (this file).
- [ ] Commit all release-prep changes to `main`.

### 3. Tagging & CI release

- [ ] Tag the release: `git tag v0.1.0` and `git push origin v0.1.0`.
- [ ] **The release workflow** (`.github/workflows/release.yml`) must trigger
      on the tag push **only** — never on branch pushes.
- [ ] Workflow cross-compiles all targets (linux/darwin/windows ×
      amd64/arm64), packages each binary as `.tar.gz` (unix) or `.zip`
      (windows), attaches a `SHA256SUMS` manifest, and creates the GitHub
      Release.
- [ ] Verify the Release on GitHub: five archives attached, correct version
      tag, and (if enabled) auto-generated release notes.

### 4. Post-release

- [ ] Confirm `tinoc version` on a downloaded archive binary reports the
      released version.
- [ ] Sanity-run one sample from the release binary (not the local build):
      `tinoc run samples/10_struct_basics.tnc`.
- [ ] If a hotfix is needed, bump to `0.1.1` and repeat from step 1.

---

### How the release pipeline works

Pushing a tag like `v0.1.0` triggers `.github/workflows/release.yml`, which:

1. Checks out the tag and sets up Go.
2. Cross-compiles `tinoc` for every target platform using `./build.sh build-all`
   (outputs land in `dist/`).
3. Archives each binary: `tinoc-<os>-<arch>.tar.gz` for unix targets,
   `tinoc-windows-amd64.zip` for Windows.
4. Creates a GitHub Release for the tag and attaches every archive.

The workflow requires write permission to `contents` (release creation) and
runs with the default `GITHUB_TOKEN`, so no extra secrets are needed.

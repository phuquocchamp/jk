# Plan: release skill, release harness, install and setup commands

Status: **executed 2026-11-16** on `feat/release-setup`, stacked on
`feat/agent-build-triage` so the first release includes the triage tools.
Items 1-6 and 8 are done; item 7 (Homebrew) was skipped as decided. The MCP
tool count is now 15, not 12.

Reference: gleanify's `.claude/skills/release/SKILL.md`, `scripts/release.mjs`,
`scripts/check-release-tag.mjs`. jk is simpler: one Go binary, no staging or
production, no store. The version lives only in the git tag.

## What jk already has

- `.goreleaser.yaml`: 6 targets (linux, darwin, windows x amd64, arm64),
  archives with `LICENSE`, `README.md`, `skills/`, `checksums.txt`,
  commit-grouped notes, an install footer.
- `.github/workflows/release.yml`: on `v*` tags runs `make lint-lite` and GoReleaser.
- `ci.yml`, `docs.yml`, `Makefile` (`release-check`, `snapshot`), `docs/.../release.md`.
- No tags yet. The first release is `v0.1.0`.

## Gaps and the work for each

### 1. Release skill: `.claude/skills/release/SKILL.md`

Based on gleanify's skill, without staging, production and the store.

1. **Preflight.** Run `git status`, check that `main == origin/main` and the
   latest `ci` run on `main` is green, then run `make all docs-check release-check`.
2. **Pick the version.** Find the last tag with `git describe --tags --abbrev=0`
   and the commits since it. Apply the SemVer rules from `docs/release.md`
   (pre-1.0: minor for new or changed CLI/JSON, patch for fixes only). Show
   `vOLD -> vNEW` and the reason, then **wait for a yes**.
3. **Tag.** Run `scripts/release.sh <patch|minor|major>`, which tags locally.
   Then **ask for a yes** and run `git push origin vX.Y.Z`.
4. **Watch.** Run `gh run list --workflow release.yml --limit 1`, then
   `gh run watch <id> --exit-status` (in the background). On failure, run
   `gh run view <id> --log-failed`.
5. **Verify.** `gh release view vX.Y.Z --json assets` must show 6 archives and
   `checksums.txt`. Download the archive for this machine, check it against the
   checksums and run `jk version`. Then run `install.sh` against the tag.
6. **Recovery.** If the run failed before publishing: delete the tag and tag
   again. If a broken release was published: never move the tag (the Go proxy
   caches it). Cut a new patch release instead.
7. **Report.** Give the release URL, the asset list, the install commands, and
   anything that was skipped.

Every push and tag is outward-facing, so get a yes before each one.

### 2. Release harness: `scripts/release.sh` + `make release BUMP=...`

A POSIX shell script with the same guards as gleanify's `release.mjs`:

- the current branch is `main`
- the working tree is clean
- after `git fetch --tags origin main`, `HEAD == origin/main`
- the next tag does not exist yet

It computes the next version from the latest `vX.Y.Z` tag (`v0.0.0` when there
is none) and the bump level, accepts `--pre rc` for `vX.Y.Z-rc.N`, and creates
an annotated tag. It **does not push**; it prints the push command. Add a
`release` target to the `Makefile`.

### 3. Tag check in CI: `release.yml`

Add a first step that fails when:

- the tag is not `vMAJOR.MINOR.PATCH[-pre]`
- the tagged commit is not an ancestor of `origin/main`

Run `make test` before GoReleaser (its `before` hook already runs the tests,
so keep only one of the two).

Optional, recommended: build provenance with `actions/attest-build-provenance`
over `dist/*.tar.gz dist/*.zip` (needs `id-token: write`, `attestations: write`).
Users can then check a download with `gh attestation verify`.
*Verify the action version and inputs via Context7 at execution.*

### 4. `install.sh` (curl | sh)

```sh
curl -fsSL https://raw.githubusercontent.com/phuquocchamp/jk/main/install.sh | sh
curl -fsSL .../install.sh | sh -s -- --version v0.1.0 --dir ~/bin
```

- Detect OS (`uname -s`: Darwin, Linux) and arch (`x86_64` -> amd64,
  `arm64`/`aarch64` -> arm64). Windows users get a pointer to the zip or
  `go install`.
- Get the latest tag from the `https://github.com/phuquocchamp/jk/releases/latest`
  redirect, not the API, which is rate-limited.
- Strip the leading `v`, because the archive name uses `{{ .Version }}`.
- Download the archive and `checksums.txt` into a `mktemp -d` directory and
  verify with `sha256sum` or `shasum -a 256`. Abort on a mismatch.
- Install to `$JK_INSTALL_DIR`, else `~/.local/bin`. Warn when that directory
  is not on `PATH`.
- No sudo. No changes to shell rc files. Do not set up MCP or the skill
  automatically; instead print the next steps (`jk auth login`, `jk setup ...`).
- Serve it raw from `main`, and also attach it as a release asset
  (`release.extra_files`) so a pinned copy exists for each version.
- Test it with `shellcheck`, plus a CI job that runs it against the latest
  release on ubuntu and macos (only after a release exists).

### 5. Setup commands in the CLI: `jk setup`

`go install` users never get the archive's `skills/` folder, so the skill is
embedded in the binary.

- **Embed.** `go:embed` cannot use `..`, so add `skills/embed.go`
  (`package skills`, `//go:embed jenkins`, `var FS embed.FS`) and import it
  from `internal/cli`. `internal/kit` must not import it (see `lint-lite`).
- **`jk setup skill [--dir ~/.claude/skills] [--force] [--dry-run]`** writes
  `jenkins/` there. It refuses to overwrite a changed copy without `--force`,
  and prints the paths it wrote.
- **`jk setup mcp [--client claude|print] [--scope user|project|local] [--tier read] [--dry-run]`**
  - `claude` (the default when `claude` is on `PATH`) runs
    `claude mcp add --scope <s> jk -- <os.Executable()> mcp [--tier ...]`.
  - `print` writes a JSON snippet for desktop apps, Cursor and others with the
    **absolute** binary path.
  - It never passes the token with `-e`, because Claude Code saves it in plain
    text (the README already warns about this).
  - *Verify the `claude mcp add` flags and scopes via Context7 or the Claude
    Code docs at execution.*
- **`jk setup`** with no subcommand prints the next steps. It does not prompt.
- **CLI contract:** only data on stdout (JSON when piped), errors with the
  existing stable codes, `--dry-run` shows the plan.
- These commands are **not** MCP tools, so the "12 operations" count stays.
- Tests go in `internal/cli/cli_test.go`: skill files are written to a temp
  directory, `--force` behaviour, and the command built for MCP (inject the
  exec function, don't run `claude`).
- Run `make docs-gen` and commit the new reference pages, otherwise
  `docs-check` fails in CI.

### 6. Docs

- `README.md` and `getting-started.mdx` Installation: install with the script
  (curl), `go install` or a download, then `jk auth login`, `jk setup skill`,
  `jk setup mcp`.
- `mcp.md`: `jk setup mcp` as the main path, `claude mcp add` as the manual way.
- `release.md`: `make release BUMP=...`, the tag check, the release skill,
  install.sh in the assets, attestation verification.
- `.goreleaser.yaml` `release.footer`: the install script line, `jk setup`.

### 7. Optional: Homebrew tap (needs your decision)

GoReleaser publishes the formula or cask to a separate repo,
`phuquocchamp/homebrew-tap`. Users install with
`brew install phuquocchamp/tap/jk`.

To set it up:

- create the tap repo
- create a fine-grained PAT with `contents: write` on that repo only
- add it as the secret `HOMEBREW_TAP_TOKEN`

*At execution, check via Context7 whether GoReleaser v2 wants `brews` or
`homebrew_casks` now.*

## Order of execution

1. Branch `feat/release-setup` off `main`.
2. `skills/embed.go` and `jk setup skill|mcp` with tests, then `make docs-gen`.
3. `scripts/release.sh` and the `make release` target.
4. `install.sh` with shellcheck, and `extra_files` in goreleaser.
5. The tag check in `release.yml` (plus attestation and brew if chosen).
6. Docs and the README.
7. The release skill.
8. Check: `make all docs-check release-check snapshot`. Run `install.sh`
   against a local `dist/` (add a `JK_BASE_URL` override for testing).
   `jk setup skill --dir $(mktemp -d)`. `jk setup mcp --dry-run`.
9. One commit per item. Open a PR. The first real release, `v0.1.0`, follows
   the new skill after merge, with a yes at each push.

## Decisions (2026-11-16)

- Homebrew tap: **not now**. Item 7 is skipped.
- Release notes: **keep the commit-grouped notes**. No `CHANGELOG.md`.
- Command name: **`jk setup skill|mcp`**.
- Build provenance attestations: **yes**.

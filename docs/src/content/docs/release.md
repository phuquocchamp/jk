---
title: "Releasing"
---

Releases are built by [GoReleaser](https://goreleaser.com) in GitHub Actions
when a `v*` tag is pushed. There is nothing to upload by hand.

## What a release contains

- `jk_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows) for linux, darwin and
  windows, amd64 and arm64. Each archive holds the static `jk` binary,
  `LICENSE`, `README.md` and the Claude Code skill in `skills/`.
- `checksums.txt` (SHA-256).
- `install.sh`, the installer pinned to this version.
- Build provenance attestations for every archive
  (`gh attestation verify <archive> -R phuquocchamp/jk`).
- Release notes made from the commits since the last tag, grouped into
  Features (`feat:`), Bug fixes (`fix:`) and Other changes. Commits starting
  with `docs:`, `test:`, `chore:` or `ci:` are left out.

The version comes from the tag and is set with
`-ldflags "-X main.version=..."`, so `jk version` prints it.

## Versioning

jk follows [semantic versioning](https://semver.org). Before 1.0, a minor
version may change the CLI or JSON output; after 1.0 that needs a major
version. Error codes and exit codes are part of the contract.

A tag with a suffix (`v0.2.0-rc.1`) is published as a pre-release.

## Steps

The `release` skill in `.claude/skills/release/` walks an agent through these
steps and asks before every push.

1. Make sure `main` is green (the `ci` workflow) and up to date locally.
2. Check locally:

    ```sh
    make all docs-check release-check   # lint, test, build, CLI reference, goreleaser check
    make snapshot                       # optional: full build into dist/ without publishing
    ```

3. Pick the bump from the commits since the last tag (`git describe --tags
   --abbrev=0`): before 1.0, `minor` for new or changed CLI/JSON, `patch` for
   fixes only. Tag locally:

    ```sh
    make release BUMP=minor          # or scripts/release.sh minor [--pre rc]
    git push origin v0.1.0           # this publishes the release
    ```

   `scripts/release.sh` refuses to tag unless you are on `main`, the tree is
   clean, `HEAD` equals `origin/main`, and the tag does not exist yet. It never
   pushes.

4. The `release` workflow checks that the tag is `vMAJOR.MINOR.PATCH[-pre]` on
   a commit in `main`, runs lint, tests and GoReleaser, attests the archives,
   then installs the release with `install.sh` on Ubuntu and macOS. Watch it:

    ```sh
    gh run list --workflow release.yml --limit 1
    gh run watch <id> --exit-status
    gh release view v0.1.0 --json assets --jq '.assets[].name'
    ```

5. `go install github.com/phuquocchamp/jk/cmd/jk@v0.1.0` and the install
   script work as soon as the release is public.

## If a release goes wrong

- **The workflow failed before publishing:** fix on `main`, delete the tag
  (`git push --delete origin v0.1.0 && git tag -d v0.1.0`) and tag again.
- **A broken release was published:** do not move the tag, since the Go module
  proxy has cached it. Publish a new patch version instead, and mark the bad
  release as a pre-release or delete it on GitHub.

## Workflows

| Workflow | Runs on | Does |
|---|---|---|
| `ci.yml` | push to `main`, pull requests | `make lint-lite shellcheck test cross docs-check` |
| `docs.yml` | changes to docs or the CLI | regenerates the CLI reference check, builds the Starlight site (fails on broken links), deploys to GitHub Pages from `main` |
| `release.yml` | `v*` tags | tag check, lint, GoReleaser release, attestations, `install.sh` on Ubuntu and macOS |

## One-time GitHub setup

- **Settings → Pages → Source:** GitHub Actions.
- **Settings → Actions → General → Workflow permissions:** the default
  read-only token is enough; the workflows ask for the extra permissions they
  need (`contents: write`, `id-token: write` and `attestations: write` for
  releases, `pages: write` and `id-token: write` for docs).
- Optional: protect `v*` tags so only maintainers can create them.

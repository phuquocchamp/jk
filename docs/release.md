# Releasing

Releases are built by [GoReleaser](https://goreleaser.com) in GitHub Actions
when a `v*` tag is pushed. There is nothing to upload by hand.

## What a release contains

- `jk_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows) for linux, darwin and
  windows, amd64 and arm64. Each archive holds the static `jk` binary,
  `LICENSE`, `README.md` and the Claude Code skill in `skills/`.
- `checksums.txt` (SHA-256).
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

1. Make sure `main` is green (the `ci` workflow) and the docs build.
2. Check locally:

    ```sh
    make all            # lint-lite, test, build
    make docs-check     # CLI reference is committed and current
    make release-check  # goreleaser check (needs goreleaser v2)
    make snapshot       # optional: full build into dist/ without publishing
    ```

3. Tag and push:

    ```sh
    git tag -a v0.1.0 -m "v0.1.0"
    git push origin v0.1.0
    ```

4. The `release` workflow runs lint, tests and GoReleaser, then publishes the
   GitHub release. Check the release page and try one archive:

    ```sh
    jk version
    ```

5. `go install github.com/phuquocchamp/jk/cmd/jk@v0.1.0` works as soon as the
   tag is public.

## If a release goes wrong

- **The workflow failed before publishing:** fix on `main`, delete the tag
  (`git push --delete origin v0.1.0 && git tag -d v0.1.0`) and tag again.
- **A broken release was published:** do not move the tag, since the Go module
  proxy has cached it. Publish a new patch version instead, and mark the bad
  release as a pre-release or delete it on GitHub.

## Workflows

| Workflow | Runs on | Does |
|---|---|---|
| `ci.yml` | push to `main`, pull requests | `make lint-lite test cross docs-check` |
| `docs.yml` | changes to docs or the CLI | regenerates the CLI reference check, `mkdocs build --strict`, deploys to GitHub Pages from `main` |
| `release.yml` | `v*` tags | lint, GoReleaser release |

## One-time GitHub setup

- **Settings → Pages → Source:** GitHub Actions.
- **Settings → Actions → General → Workflow permissions:** the default
  read-only token is enough; the workflows ask for the extra permissions they
  need (`contents: write` for releases, `pages: write` and `id-token: write`
  for docs).
- Optional: protect `v*` tags so only maintainers can create them.

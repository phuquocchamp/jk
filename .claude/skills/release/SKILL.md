---
name: release
description: Release jk — cut a new version tag, publish it through the release workflow and verify the assets. Use when the user asks to release, ship, publish or tag a new jk version, or to check that a release worked.
---

# Release jk

A release is a `vX.Y.Z` tag on `main`. Pushing the tag runs
`.github/workflows/release.yml`, which checks the tag, runs lint and tests,
builds with GoReleaser, attests the archives, publishes the GitHub release and
installs it with `install.sh` on Ubuntu and macOS. The version lives only in
the tag.

**Every push is outward-facing.** Get an explicit yes from the user before
`git push` of a branch and before `git push origin vX.Y.Z`. A yes for one push
is not a yes for the next.

## 1. Preflight

```sh
git status --short                      # must be empty
git fetch origin main --tags
git rev-parse main origin/main          # must be equal
gh run list --workflow ci.yml --branch main --limit 1   # must be success
make all docs-check release-check       # lint, test, build, CLI reference, goreleaser check
```

Stop and report if any check fails. Do not tag a red `main`.

## 2. Pick the version

```sh
git describe --tags --abbrev=0 2>/dev/null || echo "no tag yet (v0.0.0)"
git log --oneline "$(git describe --tags --abbrev=0 2>/dev/null)"..HEAD  # all commits when there is no tag
```

SemVer, from `docs/src/content/docs/release.md`:

- before 1.0: `minor` when the CLI, JSON output, error codes or MCP tools are
  new or changed (`feat`, `feat!`, `BREAKING CHANGE`); `patch` for fixes only.
- after 1.0: breaking changes need `major`.
- a release candidate: add `--pre rc` (`vX.Y.Z-rc.N`, published as a pre-release).

Show `vOLD -> vNEW`, the bump and the commits that decided it. **Wait for a yes.**

## 3. Tag and push

```sh
make release BUMP=minor        # or: scripts/release.sh minor [--pre rc] [--dry-run]
```

The script tags locally only after checking the branch, a clean tree,
`HEAD == origin/main` and that the tag is new. It prints the push command.
**Ask for a yes**, then:

```sh
git push origin vX.Y.Z
```

## 4. Watch the workflow

```sh
gh run list --workflow release.yml --limit 1
gh run watch <id> --exit-status        # run it in the background; it takes a few minutes
gh run view <id> --log-failed          # on failure
```

## 5. Verify

```sh
gh release view vX.Y.Z --json assets --jq '.assets[].name'
```

Expect 6 archives (darwin, linux, windows × amd64, arm64), `checksums.txt` and
`install.sh`. Then on this machine:

```sh
dir=$(mktemp -d)
curl -fsSL https://raw.githubusercontent.com/phuquocchamp/jk/main/install.sh | sh -s -- --version vX.Y.Z --dir "$dir"
"$dir/jk" version
gh release download vX.Y.Z --pattern 'jk_*_darwin_arm64.tar.gz' --dir "$dir"
gh attestation verify "$dir"/jk_*_darwin_arm64.tar.gz -R phuquocchamp/jk
GOFLAGS=-mod=mod go install github.com/phuquocchamp/jk/cmd/jk@vX.Y.Z   # optional
```

The `install` job of the release workflow must be green too.

## 6. Recovery

- **The workflow failed before publishing** (no release on GitHub): fix on
  `main`, then delete and recreate the tag — ask before both pushes:
  `git push --delete origin vX.Y.Z && git tag -d vX.Y.Z`, then step 3 again.
- **A broken release was published:** never move or reuse the tag; the Go
  module proxy has cached it. Fix on `main` and cut a new patch release. Mark
  the bad one as a pre-release or delete it on GitHub if the user agrees.

## 7. Report

Give the release URL, the asset list, the install commands
(`install.sh`, `go install`), the result of each verification, and anything
that was skipped or failed.

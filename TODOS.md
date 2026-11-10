# TODOS

## Documentation site (MkDocs Material on GitHub Pages)

Goal: publish docs at `https://phuquocchamp.github.io/jk/`, rebuilt on every push to `main`.

### Setup
- [ ] Push the repo to GitHub (the `origin` remote is set; the docs and release work is not pushed yet)
- [ ] In GitHub, set **Settings → Pages → Source** to **GitHub Actions**
- [x] Add `mkdocs.yml` (Material theme, search, dark mode, code copy, nav)
- [x] Add `docs/requirements.txt` pinning `mkdocs` and `mkdocs-material`

### Generated CLI reference
- [x] Add a generator (`cmd/gendocs/main.go`) using `github.com/spf13/cobra/doc` to write one Markdown page per command into `docs/reference/`
- [x] Add a `make docs` target (generate the reference, then `mkdocs build --strict`)
- [x] Add a `make docs-serve` target for local preview (`mkdocs serve`)

### Content
- [x] `docs/index.md`: overview, adapted from README
- [x] `docs/getting-started.md`: install, first command
- [x] `docs/authentication.md`: API token, env vars, keyring
- [x] `docs/configuration.md`: config file and profiles
- [x] `docs/policy.md`: read / write / danger tiers, allow/deny globs
- [x] `docs/tools.md`: tools grouped by area, what is not supported, limitations
- [x] `docs/mcp.md`: running `jk mcp` and wiring it into agents
- [x] `docs/errors.md`: output, stable error codes and exit codes
- [x] `docs/release.md`: release process
- [x] Link the existing `docs/design.md` in the nav

### CI / deploy
- [x] Add `.github/workflows/docs.yml`: on push to `main`, check the reference, build with MkDocs, deploy with `actions/upload-pages-artifact` + `actions/deploy-pages`
- [x] Fail CI if `docs/reference/` is out of date with the CLI (`make docs-check`)

### Optional
- [ ] Add `anthropics/claude-code-action` so PRs that touch `internal/cli` get a docs update or review
- [x] Add the docs link to the README
- [ ] Add the docs link to the GitHub repo "About" box

## Branding
- [x] Icon and banner in `docs/assets/` (original artwork, Jenkins palette)
- [ ] Pick the final icon/banner variant from the design canvas
- [ ] Upload a social preview image (Settings → General → Social preview; GitHub needs PNG/JPG, 1280×640)

## Release
- [x] `.github/workflows/ci.yml` (lint, test, cross-compile, docs check)
- [x] `.github/workflows/release.yml` (GoReleaser on `v*` tags)
- [x] Grouped changelog, pre-release detection, install footer in `.goreleaser.yaml`
- [ ] Run `make release-check` locally once goreleaser v2 is installed
- [ ] Tag the first release (`v0.1.0`)
- [ ] Optional: Homebrew tap (GoReleaser `homebrew_casks`), needs a tap repo and a token

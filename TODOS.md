# TODOS

## Documentation site (MkDocs Material on GitHub Pages)

Goal: publish docs at `https://phuquocchamp.github.io/jk/`, rebuilt on every push to `main`.

### Setup
- [ ] Push the repo to GitHub (no remote is configured yet)
- [ ] In GitHub, set **Settings → Pages → Source** to **GitHub Actions**
- [ ] Add `mkdocs.yml` (Material theme, search, dark mode, code copy, nav)
- [ ] Add `docs/requirements.txt` pinning `mkdocs-material`

### Generated CLI reference
- [ ] Add a small generator (e.g. `cmd/gendocs/main.go`) using `github.com/spf13/cobra/doc` to write one Markdown page per command into `docs/reference/`
- [ ] Add a `make docs` target (generate the reference, then `mkdocs build --strict`)
- [ ] Add a `make docs-serve` target for local preview (`mkdocs serve`)

### Content
- [ ] `docs/index.md`: overview, adapted from README
- [ ] `docs/getting-started.md`: install, first command
- [ ] `docs/authentication.md`: API token, env vars, keyring
- [ ] `docs/policy.md`: read-only / trigger / stop policy levels
- [ ] `docs/mcp.md`: running `jk mcp` and wiring it into agents
- [ ] `docs/errors.md`: stable error codes and exit codes
- [ ] Link the existing `docs/design.md` in the nav

### CI / deploy
- [ ] Add `.github/workflows/docs.yml`: on push to `main`, regenerate the reference, build with MkDocs, deploy with `actions/upload-pages-artifact` + `actions/deploy-pages`
- [ ] Fail CI if `docs/reference/` is out of date with the CLI

### Optional
- [ ] Add `anthropics/claude-code-action` so PRs that touch `internal/cli` get a docs update or review
- [ ] Add the docs link to the README and the GitHub repo "About" box

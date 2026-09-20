# Requirement Updater — Design Document

**Application:** go-update-requirements  
**Version:** 1.2.0  
**Language:** Go 1.22.1  
**License:** Apache 2.0

## 1. Purpose

Requirement Updater is a command-line tool that refreshes Python `requirements.txt` files to the latest versions available to `pip`, including git-installed packages pinned to the latest version tag.

It does this by:

1. Stripping pinned `==` versions from a requirements file.
2. Resolving each `package @ git+...` line to the repository’s latest version tag.
3. Uninstalling the packages currently present in the active Python environment.
4. Reinstalling from the updated requirements file (so pip resolves the newest compatible PyPI versions and the selected git tags).
5. Writing `pip freeze` back into the original file, then restoring git refs to the tags that were installed (freeze otherwise writes commit SHAs).

The intended user is someone maintaining Python projects who wants a frozen, fully-pinned `requirements.txt` that reflects current PyPI versions and the latest tagged git dependencies.

## 2. Goals and non-goals

### Goals

- Update one or more requirements files in place.
- When no files are named, discover every `requirements.txt` under the current working directory.
- Move git-based requirement lines (`package @ git+...`) to the latest version tag, preserving URL fragments such as `#subdirectory=...`.
- Fail fast with a clear message when a target file is missing, not regular, or not readable/writable.
- Print coloured progress so a terminal run is easy to follow.

### Non-goals

- Resolving or rewriting PEP 621 / `pyproject.toml` / Poetry / Pipenv lock files.
- Isolating a virtual environment; the tool uses whatever `pip` is first on `PATH`.
- Partial upgrades (for example “only FastAPI” or “only minor versions”).
- Preserving comments, extras, environment markers, or constraint operators other than `==`.
- Following a git default branch when a repository has no version tags.
- Concurrent processing of multiple files, or a dry-run / rollback mode.

## 3. High-level architecture

The program is a thin CLI over a local library module. PyPI work is done by spawning `pip`. Git tag discovery is done by spawning `git ls-remote`; the Go code never talks to PyPI or a git hosting API itself.

```
┌─────────────────────────────────────────────────────────────┐
│  CLI  (main.go)                                             │
│  module: schleising.net/update-requirements                 │
│                                                             │
│  • parse -v / usage                                         │
│  • choose filenames (args or recursive discovery)           │
│  • call updater.UpdateRequirements for each file            │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  Library  (updater/)                                        │
│  module: schleising.net/updater  (replace => ./updater)     │
│                                                             │
│  FindRequirements()                                         │
│  UpdateRequirements(filename)                               │
│      ├── validate file                                      │
│      ├── removeVersions()        strip "==..." pins         │
│      ├── updateGitDependencies() git ls-remote latest tag   │
│      ├── uninstallPackages()     pip freeze + uninstall -y  │
│      ├── installPackages()       pip install -r + freeze    │
│      └── applyGitTags()          restore tags over SHAs     │
└───────────────┬─────────────────────────────┬───────────────┘
                │                             │
                ▼                             ▼
         pip (system / venv)           git ls-remote
```

Runtime Go dependencies:

- `github.com/fatih/color` for terminal output
- `golang.org/x/mod/semver` for version-tag comparison

## 4. Module layout

| Path | Role |
| --- | --- |
| `main.go` | Process entry point, flags, argument handling, per-file success/failure messages |
| `updater/updater.go` | Discovery, validation, `==` stripping, and pip orchestration |
| `updater/git.go` | Git requirement parsing, latest-tag selection, and post-freeze retagging |
| `updater/git_test.go` | Unit tests for git URL parsing, tag picking, and file rewrites |
| `updater/go.mod` | Nested module `schleising.net/updater` |
| `go.mod` | Root module; `replace schleising.net/updater => ./updater` |
| `tests/requirements.txt` | Sample frozen file used as a manual fixture (includes git-installed packages) |
| `LICENSE` | Apache 2.0 |

The nested-module + `replace` pattern keeps the updater package independently versionable while still building as one binary from the repo root.

Application version is a constant in `main.go` (`application_version`), not derived from git tags or `go.mod`.

## 5. Command-line interface

```
Requirement Updater Version: 1.2.0

Usage: update-requirements [filename...]
  -v    Print the version and exit
```

| Invocation | Behaviour |
| --- | --- |
| `update-requirements -v` | Print version and exit 0 |
| `update-requirements` | Recursively find every file named `requirements.txt` from the current working directory, then update each |
| `update-requirements path/to/file.txt [more...]` | Update the named files in argument order |

Discovery only matches the exact basename `requirements.txt`. Explicit arguments may be any path; they are not required to use that name.

A missing argument list is detected with `len(os.Args) < 2`. Because Go’s `flag` package consumes `-v` from `os.Args`, the no-argument path is also taken when the only extra token is `-v` — which is already handled before discovery.

## 6. Core algorithm

`UpdateRequirements` is the only public mutation entry point. It is strictly sequential and in-place.

### 6.1 File validation

Before any rewrite, `os.Stat` must succeed and the path must be:

- a regular file (not a directory or special node)
- owner-writable (`0200`)
- owner-readable (`0400`)

Any failure returns immediately so the CLI can print a specific “does not exist” message via `os.IsNotExist`.

### 6.2 Version stripping (`removeVersions`)

The file is opened read/write, scanned line by line, and rewritten in place:

- Each line is split on the first `==`.
- If a pin is present, only the text before `==` is kept.
- Otherwise the line is kept unchanged (comments, blanks, `package @ git+...`, `>=` / `~=` pins, etc.).

The file is then truncated and rewritten with a trailing newline on every line.

This step exists so that a subsequent `pip install -r` is not constrained to the old PyPI pins. Git lines are left intact here and rewritten in the next step.

### 6.3 Git tag resolution (`updateGitDependencies`)

Each `name @ git+...` line is parsed as a PEP 508 direct URL:

- Supported URL forms include `git+https://`, `git+ssh://git@host/...`, and scp-like `git+git@host:path`.
- Credentials in the URL (`user:token@`) are preserved.
- A VCS ref, if present, is the last `@` that is not the SSH/userinfo `@`.
- URL fragments such as `#subdirectory=py_client` are preserved.

For each such line the tool runs:

```
git ls-remote --tags --refs <repo-url>
```

with a 30-second timeout, where `<repo-url>` is the git+ URL with the `git+` prefix removed.

**Latest tag** means the highest stable semantic version among the remote tags:

1. Tags that parse as semver (optional leading `v`, for example `v1.2.3` or `1.2.3`) are considered.
2. Non-semver tags (`nightly`, `release`, …) are ignored.
3. Stable tags win over newer prereleases (`v1.9.0` beats `v2.0.0-rc.1`).
4. If only prerelease tags exist, the highest prerelease is used.
5. If the repository has no version tags, a warning is printed and the existing git ref is left unchanged.

On success the line is rewritten to `@<latest-tag>` before pip runs, so install fetches that tag rather than the previous commit SHA.

`git ls-remote` failures (missing `git`, network error, private repo without credentials) abort the update for that file.

### 6.4 Uninstall (`uninstallPackages`)

1. Run `pip freeze`.
2. For each non-empty line, cut on `" @ "` so git-installed packages become a bare distribution name (`md_mermaid @ git+...` → `md_mermaid`).
3. If any packages remain, run a single `pip uninstall -y <pkg...>`.

This uninstalls **every** package reported by the current `pip`, not only the packages listed in the requirements file.

### 6.5 Install and refreeze (`installPackages`)

1. Run `pip install -r <filename>` against the unpinned, git-tagged file. Pip resolves the latest PyPI versions and the selected git tags.
2. Recreate the same file and run `pip freeze` with stdout redirected into it.

The resulting freeze is a full dump of the environment after that install, including transitive dependencies. Git packages in freeze output are typically pinned to a commit SHA, not the tag name.

### 6.6 Restore git tags (`applyGitTags`)

After freeze, each git line whose package name matches a successfully resolved tag is rewritten so the ref is that tag again (fragments are kept). Package names are compared after PEP 503-style normalisation (lowercase, `_` → `-`).

This keeps the requirements file readable and makes the next install request the same tag rather than a detached SHA.

## 7. Data flow

```
requirements.txt
    Django==4.2.0
    requests==2.31.0
    md_mermaid @ git+https://github.com/schleising/md_mermaid@<old-sha>
            │
            ▼  removeVersions
    Django
    requests
    md_mermaid @ git+https://github.com/schleising/md_mermaid@<old-sha>
            │
            ▼  git ls-remote --tags --refs  →  latest version tag
    Django
    requests
    md_mermaid @ git+https://github.com/schleising/md_mermaid@v1.2.3
            │
            ▼  pip uninstall -y  (entire environment)
            │
            ▼  pip install -r
            │
            ▼  pip freeze > requirements.txt
    Django==5.x.y
    requests==2.x.y
    md_mermaid @ git+https://github.com/schleising/md_mermaid@<sha-of-v1.2.3>
    <transitive packages also frozen>
            │
            ▼  applyGitTags
    Django==5.x.y
    requests==2.x.y
    md_mermaid @ git+https://github.com/schleising/md_mermaid@v1.2.3
    <transitive packages also frozen>
```

## 8. Error handling and exit behaviour

- Discovery errors (including “no requirements.txt files found”) print in red and exit 1.
- Per-file errors print in red and **exit 1 immediately**. Remaining files in the argument list are not processed.
- Missing files get a dedicated message; other errors print `err.Error()`.
- Successful files print `<filename> updated successfully` in bold green.
- A git dependency with no version tags prints a yellow warning and does not fail the run.

Pip and git command failures (`exec.Command` / `Run` / `Output`) are returned as-is, with `git ls-remote` stderr included when present. There is no retry, and pip’s stderr is not captured for display.

There is also no backup of the original file. If install fails after versions have been stripped, git refs rewritten, or packages uninstalled, the requirements file and the environment may be left in a partial state.

## 9. Important behavioural constraints

These are load-bearing properties of the current design, not incidental bugs:

1. **Environment-wide uninstall.** The tool assumes the active `pip` belongs to a dedicated virtualenv (or a disposable environment). Running it against a shared or system Python will uninstall unrelated packages.
2. **Freeze is environment-wide.** After install, the written file is `pip freeze` of *everything* installed, not a round-trip of the original package list. Extra packages already in the env, and newly pulled transitive deps, land in the file.
3. **Multiple files in one run share one environment.** Files are processed sequentially, and each cycle uninstalls everything then freeze-writes that file. Updating two requirements files in one invocation will leave the first file reflecting only the second file’s install (and vice versa as processing continues). The safe usage for multiple files is one file per environment, or one invocation per project/venv.
4. **Only `==` pins are stripped.** Operators such as `>=`, `~=`, `!=`, and comma-combined specifiers are left intact and will continue to constrain pip.
5. **Discovery is unbounded.** `filepath.Walk` from the cwd does not skip `.git`, `.venv`, `node_modules`, or other well-known directories. A `requirements.txt` inside a virtualenv will be treated as a target.
6. **`pip` and `git` must be on `PATH`.** There is no `python -m pip` fallback, no configurable pip/git binary, and no GitHub/GitLab API client.
7. **Latest git ref is the latest version tag, not HEAD.** Repositories that only move a branch, or that tag with non-semver names, are not advanced (aside from a warning when no version tags exist).
8. **Editable (`-e`) and egg-only git lines are not rewritten.** Only `name @ git+...` direct URL references are treated as git dependencies.

## 10. External commands

| Step | Command | Captured |
| --- | --- | --- |
| List git tags | `git ls-remote --tags --refs <repo-url>` | stdout (parsed) |
| List installed | `pip freeze` | stdout (parsed) |
| Uninstall | `pip uninstall -y <names...>` | none (success/fail only) |
| Install latest | `pip install -r <file>` | none |
| Refreeze | `pip freeze` | stdout redirected onto `<file>` |

The tool therefore inherits pip’s resolver, index configuration (`PIP_INDEX_URL`, `pip.conf`), and git-URL support, plus git’s own credentials for `ls-remote`. Network access, credentials, and TLS behaviour are those of pip and git.

Git VCS lines look like:

```
md_mermaid @ git+https://github.com/schleising/md_mermaid@<tag-or-sha>
notify-run @ git+https://github.com/notify-run/notify.run.git@<tag-or-sha>#subdirectory=py_client
```

Uninstall normalises these via `" @ "`. Install uses the rewritten tag URL. After freeze, `applyGitTags` puts the tag name back so the file does not stay SHA-pinned.

## 11. Testing posture

`updater/git_test.go` covers git URL parsing (https, ssh, credentials, fragments), semver tag selection (including prerelease vs stable), rewriting a requirements file to latest tags, and restoring tags after a freeze-style SHA dump. `git ls-remote` is injected in tests so they do not need the network.

There is no end-to-end coverage of pip. `tests/requirements.txt` (and a copy) remain manual fixtures representing a real FastAPI-era freeze, including two git-sourced packages.

Further tests would still be useful for:

- validation errors (missing, directory, permission bits)
- discovery of nested `requirements.txt` and the empty-tree error
- uninstall argument construction for `pkg @ git+...` lines
- CLI exit codes for `-v`, no files found, and mid-loop failure

## 12. Build and runtime requirements

- Go 1.22.1 or newer to build.
- A `pip` executable on `PATH` at runtime, typically from an activated virtualenv.
- A `git` executable on `PATH` at runtime, used to list remote tags.
- Network access for `pip install` and `git ls-remote`, unless a local index / git remote is configured.

Build from the repo root:

```
go build -o update-requirements .
```

The root `go.mod` replace directive is required; the updater module is not published independently.

## 13. Future work (out of scope for the current implementation)

These would be natural extensions if the tool grows beyond a personal utility:

- Operate on a named venv or `python -m pip` instead of ambient `pip`.
- Skip well-known directories during discovery.
- Uninstall/install only packages named in the target file, rather than the whole environment.
- Write a freeze of just the requested packages (or use `pip freeze --requirement`) so transitive noise is optional.
- Backup / dry-run / restore on failure.
- Honour other version specifiers, extras, and environment markers.
- Continue past a failed file when several paths are given.
- Fall back to the remote default branch when a git dependency has no version tags.
- Support editable (`-e git+...`) requirement lines.
- Automated tests with a fake pip runner.

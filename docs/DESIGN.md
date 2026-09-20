# Requirement Updater — Design Document

**Application:** go-update-requirements  
**Version:** 1.1.7  
**Language:** Go 1.22.1  
**License:** Apache 2.0

## 1. Purpose

Requirement Updater is a command-line tool that refreshes Python `requirements.txt` files to the latest versions available to `pip`.

It does this by:

1. Stripping pinned versions from a requirements file.
2. Uninstalling the packages currently present in the active Python environment.
3. Reinstalling from the unpinned requirements file (so pip resolves the newest compatible versions).
4. Writing `pip freeze` back into the original file.

The intended user is someone maintaining Python projects who wants a frozen, fully-pinned `requirements.txt` that reflects current PyPI (and git) package versions.

## 2. Goals and non-goals

### Goals

- Update one or more requirements files in place.
- When no files are named, discover every `requirements.txt` under the current working directory.
- Preserve git-based requirement lines (`package @ git+...`) during version stripping.
- Fail fast with a clear message when a target file is missing, not regular, or not readable/writable.
- Print coloured progress so a terminal run is easy to follow.

### Non-goals

- Resolving or rewriting PEP 621 / `pyproject.toml` / Poetry / Pipenv lock files.
- Isolating a virtual environment; the tool uses whatever `pip` is first on `PATH`.
- Partial upgrades (for example “only FastAPI” or “only minor versions”).
- Preserving comments, extras, environment markers, or constraint operators other than `==`.
- Concurrent processing of multiple files, or a dry-run / rollback mode.

## 3. High-level architecture

The program is a thin CLI over a local library module. All pip work is done by spawning the `pip` executable; the Go code never talks to PyPI itself.

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
│  Library  (updater/updater.go)                              │
│  module: schleising.net/updater  (replace => ./updater)     │
│                                                             │
│  FindRequirements()                                         │
│  UpdateRequirements(filename)                               │
│      ├── validate file                                      │
│      ├── removeVersions()     rewrite file, strip "==..."   │
│      ├── uninstallPackages()  pip freeze + pip uninstall -y │
│      └── installPackages()    pip install -r + pip freeze   │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
                   pip (system / venv)
```

`github.com/fatih/color` is used only for terminal output. There are no other runtime dependencies.

## 4. Module layout

| Path | Role |
| --- | --- |
| `main.go` | Process entry point, flags, argument handling, per-file success/failure messages |
| `updater/updater.go` | Discovery, validation, file rewrite, and pip orchestration |
| `updater/go.mod` | Nested module `schleising.net/updater` |
| `go.mod` | Root module; `replace schleising.net/updater => ./updater` |
| `tests/requirements.txt` | Sample frozen file used as a manual fixture (includes git-installed packages) |
| `LICENSE` | Apache 2.0 |

The nested-module + `replace` pattern keeps the updater package independently versionable while still building as one binary from the repo root.

Application version is a constant in `main.go` (`application_version`), not derived from git tags or `go.mod`.

## 5. Command-line interface

```
Requirement Updater Version: 1.1.7

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

This step exists so that a subsequent `pip install -r` is not constrained to the old pins.

### 6.3 Uninstall (`uninstallPackages`)

1. Run `pip freeze`.
2. For each non-empty line, cut on `" @ "` so git-installed packages become a bare distribution name (`md_mermaid @ git+...` → `md_mermaid`).
3. If any packages remain, run a single `pip uninstall -y <pkg...>`.

This uninstalls **every** package reported by the current `pip`, not only the packages listed in the requirements file.

### 6.4 Install and refreeze (`installPackages`)

1. Run `pip install -r <filename>` against the now-unpinned file. Pip resolves the latest versions (and git refs as specified).
2. Recreate the same file and run `pip freeze` with stdout redirected into it.

The resulting file is a full freeze of the environment after that install, including transitive dependencies that were not originally listed.

## 7. Data flow

```
requirements.txt
    Django==4.2.0
    requests==2.31.0
    md_mermaid @ git+https://...
            │
            ▼  removeVersions
    Django
    requests
    md_mermaid @ git+https://...
            │
            ▼  pip uninstall -y  (entire environment)
            │
            ▼  pip install -r
            │
            ▼  pip freeze > requirements.txt
    Django==5.x.y
    requests==2.x.y
    md_mermaid @ git+https://...
    <transitive packages also frozen>
```

## 8. Error handling and exit behaviour

- Discovery errors (including “no requirements.txt files found”) print in red and exit 1.
- Per-file errors print in red and **exit 1 immediately**. Remaining files in the argument list are not processed.
- Missing files get a dedicated message; other errors print `err.Error()`.
- Successful files print `<filename> updated successfully` in bold green.

Pip command failures (`exec.Command` / `Run` / `Output`) are returned as-is. There is no retry, timeout, or capture of pip’s stderr for display.

There is also no backup of the original file. If install fails after versions have been stripped (or after uninstall), the requirements file and the environment may be left in a partial state.

## 9. Important behavioural constraints

These are load-bearing properties of the current design, not incidental bugs:

1. **Environment-wide uninstall.** The tool assumes the active `pip` belongs to a dedicated virtualenv (or a disposable environment). Running it against a shared or system Python will uninstall unrelated packages.
2. **Freeze is environment-wide.** After install, the written file is `pip freeze` of *everything* installed, not a round-trip of the original package list. Extra packages already in the env, and newly pulled transitive deps, land in the file.
3. **Multiple files in one run share one environment.** Files are processed sequentially, and each cycle uninstalls everything then freeze-writes that file. Updating two requirements files in one invocation will leave the first file reflecting only the second file’s install (and vice versa as processing continues). The safe usage for multiple files is one file per environment, or one invocation per project/venv.
4. **Only `==` pins are stripped.** Operators such as `>=`, `~=`, `!=`, and comma-combined specifiers are left intact and will continue to constrain pip.
5. **Discovery is unbounded.** `filepath.Walk` from the cwd does not skip `.git`, `.venv`, `node_modules`, or other well-known directories. A `requirements.txt` inside a virtualenv will be treated as a target.
6. **`pip` must be on `PATH`.** There is no `python -m pip` fallback and no configurable pip binary.

## 10. External interface to pip

| Step | Command | Captured |
| --- | --- | --- |
| List installed | `pip freeze` | stdout (parsed) |
| Uninstall | `pip uninstall -y <names...>` | none (success/fail only) |
| Install latest | `pip install -r <file>` | none |
| Refreeze | `pip freeze` | stdout redirected onto `<file>` |

The tool therefore inherits pip’s resolver, index configuration (`PIP_INDEX_URL`, `pip.conf`), and git-URL support. Network access, credentials, and TLS behaviour are all pip’s.

Git VCS lines in the sample fixture look like:

```
md_mermaid @ git+https://github.com/schleising/md_mermaid@<sha>
notify-run @ git+https://github.com/notify-run/notify.run.git@<sha>#subdirectory=py_client
```

Uninstall normalises these via `" @ "`; install leaves the original VCS URL in the unpinned file so pip reinstalls from git.

## 11. Testing posture

There is no `*_test.go` coverage today. `tests/requirements.txt` (and a copy) are manual fixtures representing a real FastAPI-era freeze, including two git-sourced packages.

A design-faithful test suite would need to mock or stub `pip` (or run against an isolated venv) and cover:

- `==` stripping vs untouched git / comment / blank lines
- validation errors (missing, directory, permission bits)
- discovery of nested `requirements.txt` and the empty-tree error
- uninstall argument construction for `pkg @ git+...` lines
- CLI exit codes for `-v`, no files found, and mid-loop failure

## 12. Build and runtime requirements

- Go 1.22.1 or newer to build.
- A `pip` executable on `PATH` at runtime, typically from an activated virtualenv.
- Network access for `pip install` unless a local index or wheel cache is configured.

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
- Automated tests with a fake pip runner.

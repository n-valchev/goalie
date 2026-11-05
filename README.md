# goalie

goalie is a set of checks that stop poor code before it merges. It is made for repos where coding agents write code. Each tool ships as its own binary, and all tools share one settings file.

| Tool | What it finds |
| --- | --- |
| [godry](#find-duplicates) | Go functions with the same structure, such as a copied function |
| [gocrap](#find-risky-functions) | Go functions that are complex and poorly tested |

godry and gocrap use only the Go standard library.

## Install goalie

Choose one method.

| Method | Command | What you need |
| --- | --- | --- |
| Go | `go install github.com/n-valchev/goalie/cmd/godry@latest github.com/n-valchev/goalie/cmd/gocrap@latest` | Go 1.25 or later |
| Nix, run once | `nix run github:n-valchev/goalie#godry -- --help`, or `#gocrap` | Nix with flakes |
| Nix, keep on your `PATH` | `nix profile add github:n-valchev/goalie` | Nix with flakes |

To check the install, run `godry --help` and `gocrap --help`. Each command prints its modes, flags, skip rules, and exit codes.

`--base` runs `git`. If you install with Go, you must have `git` on your `PATH`. The Nix package includes `git` and uses it only when your `PATH` has no `git`.

To add goalie to your own Nix package set, use the overlay:

```nix
{
  inputs.goalie.url = "github:n-valchev/goalie";

  # In your NixOS or nixpkgs configuration:
  # nixpkgs.overlays = [ goalie.overlays.default ];
  # After that, the package is pkgs.goalie.
}
```

## Find duplicates

godry finds Go functions with the same structure. It compares functions across the whole repository, not one package at a time.

A renamed copy still matches. godry ignores the names of local variables, the names of fields, and the values of literals. It keeps the names of called functions and the operators, so functions that do different work don't match.

godry has two modes.

| Mode | Command | Compares | Reports |
| --- | --- | --- | --- |
| Whole repo | `godry [paths]` | Every function with every other function | All pairs at or above the threshold |
| New code | `godry --base <rev> [paths]` | Each new or changed function with every scanned function | Only pairs with at least one new or changed function |

Use whole-repo mode to see the duplication that already exists. It doesn't need Git.

Use new-code mode to check a change. Pairs of old functions don't appear, so existing duplication doesn't block unrelated work. For example, to check your branch against `main`:

```sh
godry --base main
```

New-code mode includes commits since the merge base, uncommitted changes, and untracked files. It marks a function as new when a changed line falls inside it. A function that you moved counts as new.

With no paths, godry uses the `paths` setting in `.goalierc`. If there is none, it scans the current directory tree. Paths limit both modes: godry never compares a function outside them.

### Read the output

godry prints one line for each pair:

```text
internal/a.go:12: duplicates internal/b.go:40 (score 0.91, literals 0.20, anchors 3)
```

The score runs from `0` to `1`, and `1.00` means the two functions have the same structure. The other two numbers describe the match. They don't decide whether godry reports a pair, unless you set `--weak-anchors`:

| Number | Meaning | How to read it |
| --- | --- | --- |
| `literals` | Literals both functions share, divided by all literals in either function. `-` means neither has one to compare. `0`, `1`, `""`, `nil`, `true`, and `false` don't count. | High: a pasted copy. Low: the same code with different data. |
| `anchors` | Different called names that both functions use. Operators don't count. A type conversion such as `string(x)` looks like a call, so it counts. | `0` with low `literals`: only the shape of the code matches, as with two `switch` mappings. |

In new-code mode, the new function comes first. A clean scan prints `No duplicates found.`

Use `--format json` to get an object with a `findings` array. Each finding has `score`, `literals` (a number, or `null` for `-`), `anchors`, `left`, and `right`. With `--shared`, a finding can also have `case`.

### Use the exit code

| Code | Meaning |
| --- | --- |
| `0` | No duplicates, or duplicates with `--warn-only` |
| `1` | Duplicates exist |
| `2` | Any error, even with `--warn-only` |

Errors include an invalid flag, a Go file that doesn't parse, a `//go:build` line that doesn't parse, an unknown revision, and `--base` outside a Git repository.

## Change what godry reports

| Flag | Default | Meaning |
| --- | --- | --- |
| `--base <rev>` | none | Turn on new-code mode against this revision |
| `--threshold N` | `0.82` | Minimum similarity score |
| `--min-lines N` | `9` | Minimum lines in a function |
| `--min-nodes N` | `20` | Minimum nodes in the normalized function |
| `--weak-anchors N` | `0` | Drop findings with fewer than N anchors and low literals. `0` drops nothing |
| `--weak-literals X` | `0.5` | Literal score below which a finding counts as low |
| `--format F` | `text` | Output format: `text` or `json` |
| `--warn-only` | off | Exit with `0` when duplicates exist |
| `--include-tests` | off | Scan `*_test.go` files |
| `--skip-tags a,b` | none | Skip files that can only build with these tags |
| `--skip-files a,b` | none | Skip files that match these patterns |
| `--shared a,b` | none | Add a case to each finding. These patterns name your shared code |
| `--help` | | Print usage |

godry has no baseline file, and its only settings files are `.goalierc` and `.goalieignore`. It writes no files.

### Keep settings in a file

To keep your directories and settings in the repo, write them in a `.goalierc` file. The file holds one JSON object:

```json
{
  "paths": ["products/billing", "products/search", "pkg"],
  "skipFiles": ["packages.go", "internal/generated/**"],
  "skipTags": ["integration"],
  "godry": {
    "threshold": 0.82,
    "weakAnchors": 1,
    "weakLiterals": 0.5
  }
}
```

Each setting is optional, and each one matches a flag. All goalie tools read this file. Settings at the top level describe the repo and apply to every tool. Settings in the `godry` object apply only to godry.

| Setting | Where | Flag | Type |
| --- | --- | --- | --- |
| `paths` | Top level | Paths on the command line | List of strings |
| `skipTags` | Top level | `--skip-tags` | List of strings |
| `skipFiles` | Top level | `--skip-files` | List of strings |
| `shared` | Top level | `--shared` | List of strings |
| `base` | `godry` | `--base` | String |
| `threshold` | `godry` | `--threshold` | Number |
| `minLines` | `godry` | `--min-lines` | Whole number |
| `minNodes` | `godry` | `--min-nodes` | Whole number |
| `weakAnchors` | `godry` | `--weak-anchors` | Whole number |
| `weakLiterals` | `godry` | `--weak-literals` | Number |
| `format` | `godry` | `--format` | `"text"` or `"json"` |
| `warnOnly` | `godry` | `--warn-only` | `true` or `false` |
| `includeTests` | `godry` | `--include-tests` | `true` or `false` |

- godry reads `.goalierc` from the directory it runs in, and from no other folder. Paths and patterns start from that directory. Run godry from the repo root in CI.
- A flag replaces the same setting in the file. Paths on the command line replace `paths`. To turn off `warnOnly` for one run, use `--warn-only=false`.
- Patterns in `skipFiles`, `--skip-files`, and `.goalieignore` all apply.
- A missing file is not an error. Invalid JSON, an unknown setting in any section, or a bad value exits with `2`.
- JSON has no comments. To record why a file is skipped, put the pattern in `.goalieignore`.
- If you set `base` in the file, every run uses new-code mode, and no flag turns it off. To keep whole-repo scans available, pass `--base` on the command line instead.

If agents work in your repo, require human approval for changes to `.goalierc` and `.goalieignore`, for example with `CODEOWNERS`. Otherwise, an agent can lower the limits to pass the check.

### Name the case of each finding

In a monorepo, the right fix for a duplicate depends on where the two functions are. To add this to each finding, name your shared code:

```sh
godry --shared 'pkg/**'
```

```text
billing/a.go:12: duplicates pkg/b.go:40 (score 0.91, literals 0.20, anchors 3, case copies-shared)
```

| Case | Where the functions are | Usual fix |
| --- | --- | --- |
| `copies-shared` | One in shared code, one in a product | Delete the copy and use the shared function |
| `across-products` | In two different products | Decide whether to move the code to shared code |
| `same-product` | Both in one product | Fix it inside that product |
| `within-shared` | Both in shared code | Merge the two functions |

- `--shared` uses the same patterns as `--skip-files`. To match a folder, write `pkg/**`, because `pkg` alone matches a file name.
- A product is the first folder in the path from the directory godry runs in, unless a shared pattern matches the file. godry has no list of products, so every other first folder counts as one, including folders such as `tools`. To leave a folder out, don't list it in `paths`, or skip it.
- A file directly in the run directory, or above it, belongs to no product. A finding with such a file has no case, unless both functions are in shared code.
- Without `--shared`, findings have no case, and the output doesn't change.
- `--shared` doesn't change which pairs godry reports, or the exit code.
- If a pattern matches no scanned file, godry prints a warning on stderr. The run continues.

### Dropping weak findings

Two functions can match on shape alone, such as two `switch` statements that map values to strings. To drop these findings, run:

```
godry --weak-anchors 1 --weak-literals 0.5
```

godry drops a finding only when both numbers are low:

| Anchors | Literals | Result | Why |
| --- | --- | --- | --- |
| Below `--weak-anchors` | Below `--weak-literals` | Dropped | Only the shape matches |
| At or above the limit | Any | Kept | The functions call the same code |
| Any | At or above the limit | Kept | The functions hold the same data |
| Any | `-` | Kept | Nothing shows that the data differs |

- A dropped finding doesn't count, so a run that drops every finding exits with `0`.
- godry prints the count on stderr, for example `godry: 12 weak findings dropped`.
- The rule applies to both modes. It runs after comparison, so it doesn't change which pairs match.
- Dropping is off by default.

### What godry skips

These rules apply to both modes, and to paths that you name:

- The directories `vendor`, `testdata`, and `node_modules`, and directories with names that start with `.` or `_`.
- `*_test.go` files, unless you set `--include-tests`.
- Files with the header `Code generated ... DO NOT EDIT.`
- Files with an `ignore` build constraint.
- Files that can only build with a tag in `--skip-tags`.
- Files that match a pattern in `--skip-files` or in `.goalieignore`.
- Pairs in one package with the same function name and the same receiver type. These are usually platform variants.

Other build constraints and `GOOS` or `GOARCH` file suffixes have no effect. The same commit gives the same result on every machine.

### Skipping files by name

Use `--skip-files` for files that repeat on purpose, such as dependency wiring in every package.

| Pattern | Matches |
| --- | --- |
| `packages.go` | `packages.go` in any folder |
| `wire_*.go` | Any file name that starts with `wire_`, in any folder |
| `internal/users/packages.go` | That one file, from the directory godry runs in |
| `internal/*/packages.go` | `packages.go` one folder below `internal`, because `*` does not cross folders |
| `internal/generated/**` | Everything below that folder, at any depth |
| `**/generated/**` | Everything below any folder named `generated` |
| `internal/**/packages.go` | `packages.go` anywhere below `internal` |

A skipped file is not read, so it can't cause a parse error. Its functions are not compared on either side of a pair. You can repeat the flag or use a comma-separated list. A bad pattern exits with `2`.

Quote patterns on the command line, so that the shell doesn't expand them: `godry --skip-files 'internal/generated/**'`.

To keep the patterns in the repo, write them in a `.goalieignore` file:

```
# Dependency wiring repeats on purpose.
packages.go
wire_*.go
internal/legacy/client.go
```

- godry reads `.goalieignore` from the directory it runs in, and from no other folder. Run it from the repo root in CI.
- Write one pattern per line. The patterns follow the table above. Blank lines and lines that start with `#` are ignored.
- `.goalieignore` and `--skip-files` both apply.
- A missing file is not an error. A bad pattern exits with `2` and names the line.
- This is not full `.gitignore` syntax. `!`, a trailing `/`, and `**` inside a longer name such as `a**b` exit with `2`, in the file and in the flag. To skip a folder, write `folder/**`.

### Exclude one function

To exclude an intentional duplicate, write a comment directly above the function:

```go
//godry:ignore The two parsers must stay independent.
func parseHeader(line string) (Header, error) {
```

godry then leaves that function out of all comparisons. The reason is required. A comment with no reason is an error, and godry exits with `2`.

## Run godry in CI

This GitHub Actions workflow fails a pull request that adds a duplicate:

```yaml
name: godry

on:
  pull_request:

jobs:
  godry:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: go install github.com/n-valchev/goalie/cmd/godry@latest
      - run: "$(go env GOPATH)/bin/godry --base origin/${{ github.base_ref }}"
```

You must keep `fetch-depth: 0`. New-code mode needs the merge base, and the default shallow clone doesn't contain it. Without the full history, godry exits with `2` and prints this error:

```text
godry: no merge base between "origin/main" and HEAD: in a shallow clone, fetch more history
```

To adopt godry without blocking work, add `--warn-only` first. Remove it when you have checked the limits on your repository.

## Find risky functions

gocrap scores each function by its complexity and its test coverage. A high score means the function is risky to change. It uses the CRAP (Change Risk Anti-Pattern) formula, where CC is cyclomatic complexity:

```text
score = CC² × (1 - coverage)³ + CC
```

For example, a function with a complexity of 6 scores 42 with no coverage, and 10.5 with 50% coverage. Full coverage leaves a score equal to the complexity, so a very complex function fails even if it is fully tested.

| Mode | Command | Reports |
| --- | --- | --- |
| All code | `gocrap [paths]` | Every function above the limit |
| New code | `gocrap --base <rev> [paths]` | New or changed functions above the limit |

```text
pkg/a.go:12: Run scores 130.2 (complexity 12, coverage 45.0%, limit 30)
```

Findings come worst first. A clean run prints `No functions over the limit.` To list every function, run `gocrap --limit 0`. Use `--format json` to get an object with a `findings` array. Each finding has `file`, `line`, `function`, `score`, `complexity`, `coverage` (a fraction from `0` to `1`), and `limit`.

| Exit code | Meaning |
| --- | --- |
| `0` | No function is over its limit, or some are and you set `--warn-only` |
| `1` | A function is over its limit |
| `2` | An error, even with `--warn-only` |

### Set the limits

Set the limits in the `gocrap` object of `.goalierc`:

```json
{
  "shared": ["pkg/**"],
  "gocrap": {
    "limit": 30,
    "sharedLimit": 15,
    "coverCommand": "make coverage"
  }
}
```

| Setting | Flag | Type | Default |
| --- | --- | --- | --- |
| `base` | `--base` | String | None |
| `limit` | `--limit` | Number | `30` |
| `sharedLimit` | `--shared-limit` | Number | The value of `limit` |
| `coverCommand` | None | String | See [Measure all your test suites](#measure-all-your-test-suites) |
| `coverProfiles` | `--cover-profiles` | List of strings | None |
| `format` | `--format` | `"text"` or `"json"` | `"text"` |
| `warnOnly` | `--warn-only` | `true` or `false` | `false` |

- A score equal to the limit passes.
- `sharedLimit` applies to files that match `shared`. Every product depends on shared code, so you can hold it to a stricter limit. `sharedLimit` without `shared` is an error.
- gocrap also uses `paths`, `skipFiles`, `skipTags`, and `.goalieignore`. See [Keep settings in a file](#keep-settings-in-a-file).
- gocrap never scores `*_test.go` files.
- To exclude one function, write `//gocrap:ignore <reason>` directly above it. `//godry:ignore` has no effect on gocrap.

### Measure all your test suites

gocrap runs one command to gather coverage. With no settings, the command is:

```sh
go test -coverpkg=./... -coverprofile="$GOCRAP_COVER_DIR/coverage.out" ./...
```

If your repo has more suites, set `coverCommand`. gocrap follows these steps:

1. It creates an empty temporary folder and sets `GOCRAP_COVER_DIR` to that path.
2. It runs your command with `sh -c`, from the directory it runs in.
3. It reads each file in that folder as a text coverage profile. It ignores subfolders.
4. It merges the profiles. A code block counts as covered if any profile covers it.

This example runs unit, integration, and end-to-end (e2e) tests:

```sh
set -e
go test -coverpkg=./... -coverprofile="$GOCRAP_COVER_DIR/unit.out" ./...
go test -tags integration -coverpkg=./... -coverprofile="$GOCRAP_COVER_DIR/integration.out" ./...

mkdir "$GOCRAP_COVER_DIR/raw"
go build -cover -coverpkg=./... -o "$GOCRAP_COVER_DIR/raw/app" ./cmd/app
GOCOVERDIR="$GOCRAP_COVER_DIR/raw" ./e2e/run.sh "$GOCRAP_COVER_DIR/raw/app"
go tool covdata textfmt -i="$GOCRAP_COVER_DIR/raw" -o="$GOCRAP_COVER_DIR/e2e.out"
```

- Keep `-coverpkg=./...`. Without it, Go counts coverage only in the package that holds the test, and code reached from other packages scores as untested.
- An e2e binary must exit normally, because a killed process writes no coverage. In a container, mount the `GOCOVERDIR` folder.
- If the command fails, gocrap exits with `2` and prints no scores. A suite that stopped early would give falsely high scores. Use `set -e` so that a failed suite fails the command.
- The command's output goes to stderr, so stdout stays the report.
- A function in no profile has 0% coverage. This includes files that no test run builds, such as files for another platform. To leave them out, use `skipFiles` or `skipTags`.
- gocrap runs the tests in new-code mode too, because any test can cover a changed function. If no function changed, it runs nothing.

If CI has already run the tests, list the files in `coverProfiles`, or pass `--cover-profiles a.out,b.out`. gocrap then runs no command. Setting both `coverCommand` and `coverProfiles` in the file is an error, but `--cover-profiles` replaces either one for a run.

gocrap supports one Go module, and it reads `go.mod` from the directory it runs in, or above it. A profile that holds a file outside that module is an error. Limit `paths` to one module if your repo holds several.

`coverCommand` can run any command. An agent can also use it to raise its scores. Require human approval for changes to `.goalierc`, for example with `CODEOWNERS`.

### Run gocrap in CI

Use the same job setup as godry, with `fetch-depth: 0`:

```yaml
      - run: go install github.com/n-valchev/goalie/cmd/gocrap@latest
      - run: "$(go env GOPATH)/bin/gocrap --base origin/${{ github.base_ref }}"
```

## Develop goalie

1. Run `nix develop` to enter a shell with Go and `git`. You can also use your own Go 1.25 or later.
2. Run `go test ./...` to run the tests.
3. Run `nix build` to build the package. The build runs the same tests.

Nix sees only the files that Git tracks. Run `git add` on a new file before you run `nix build`.

## Credits

gocrap follows [crap4go](https://github.com/unclebob/crap4go), and shares no code with it. Scores can differ from crap4go's scores, because gocrap does not count `default` clauses as branches.

godry follows the method of [dryer](https://github.com/unclebob/dryer) and [dry4clj](https://github.com/unclebob/dry4clj). Scores differ from dryer's scores, because godry parses code with `go/parser` and the two parsers build different trees.

## License

goalie uses the [0BSD license](LICENSE).

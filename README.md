# goalie

goalie is a set of checks that stop poor code before it merges. It is made for repos where coding agents write code. Each tool ships as its own binary, and all tools share one settings file.

| Tool | What it finds |
| --- | --- |
| [godry](#find-duplicates) | Go functions with the same structure, such as a copied function |
| [gocrap](#find-risky-functions) | Go functions that are complex and poorly tested |
| [godeps](#map-your-packages) | The shape of a Go module, as a map in one HTML file |

All goalie tools use only the Go standard library.

## Install goalie

Choose one method.

| Method | Command | What you need |
| --- | --- | --- |
| Go | `go install github.com/n-valchev/goalie/cmd/godry@latest github.com/n-valchev/goalie/cmd/gocrap@latest github.com/n-valchev/goalie/cmd/godeps@latest` | Go 1.25 or later |
| Nix, run once | `nix run github:n-valchev/goalie#godry -- --help`, or `#gocrap`, or `#godeps` | Nix with flakes |
| Nix, keep on your `PATH` | `nix profile add github:n-valchev/goalie` | Nix with flakes |

To check the install, run `godry --help`, `gocrap --help`, and `godeps --help`. Each command prints its flags, settings, and exit codes.

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

## Map your packages

godeps reads your Go code and writes a map of it as one HTML file. You open the file in a browser, with no network and no install. It helps you see which code depends on a package, which imports break a rule, and where the risky and copied code is.

To make a map of the module in the current folder:

```sh
godeps > map.html
```

godeps checks nothing. arch-go stays your rule check in CI, and gocrap and godry stay your other checks. godeps only shows their reports on the map. Each report is optional.

### Read the map

The first screen shows one box for each product and one box for the shared code. A product is the first folder in a path. A class in the root folder of the module shows as its own box.

| To do this | Do this |
| --- | --- |
| Open a box or a package | Double-click it |
| Go up one level | Press Esc |
| Jump to a level above | Click the breadcrumb |
| Share a level, or keep it on reload | Copy the URL, which holds the level |
| Read why an arrow is red | Hover over the arrow, or click it to keep the panel open |
| Open the source of a type or a function | Click its link on the card of the package |

| What you see | What it means |
| --- | --- |
| Arrow | The box at the tail imports the box at the head |
| Red, dashed arrow | An import in that arrow breaks an `arch-go.yml` rule |
| Dotted arrow, marked `loop` | This arrow closes an import loop |
| Oval | A module that you listed in `foreign` |
| Green box | The worst function is at or below half its limit |
| Yellow box | The worst function is at or below its limit |
| Red, dashed box, marked `!` | The worst function is above its limit, or the gocrap report left it out |
| Grey box | You gave no gocrap report |
| `2 duplicated functions` | Two functions in the box have a duplicate. With a godry report, `0` prints too. Boxes at the edge of an opened box show this count only in their tooltip |

A function that the gocrap report leaves out is red, so that unmeasured code does not look safe. This includes a function with `//gocrap:ignore`, because godeps doesn't read comments. An assembly stub has no grade, because gocrap can't score it.

### Add the reports

Run these commands from the repo root. godeps expects the paths in the reports to start there:

```sh
arch-go --json || true
gocrap --limit 0 --shared-limit 0 --warn-only --format json > crap.json
godry --format json --warn-only > dry.json
godeps --arch-go-report .arch-go/report.json --crap-report crap.json --dry-report dry.json > map.html
```

| Command part | Why you need it |
| --- | --- |
| `\|\| true` after `arch-go` | arch-go exits with `1` when it finds a break, and it still writes the report. A script that stops on errors would otherwise never reach godeps. Keep your own arch-go step in CI as the gate |
| `--limit 0` | Without it, gocrap reports only functions above the limit, and godeps shows all the others as red |
| `--shared-limit 0` | Without it, shared functions keep their `sharedLimit` and drop out of the report. Leave this flag out if you set no `shared` patterns, because gocrap then rejects it |
| `--warn-only` | Without it, gocrap and godry exit with `1` when they report anything |

**Don't set `base` in `.goalierc` for a map.** A `base` setting in the `gocrap` or `godry` object limits that report to new code, and no flag turns it off. With gocrap, every other function then shows as red. With godry, duplicates in old code show no mark. To check new code, pass `--base` on the command line instead.

godeps takes the limits from `limit` and `sharedLimit` in the `gocrap` object of `.goalierc`, or `30` if you set no `limit`. It ignores the limit in the report, which is `0` in this run. godeps does not check these two values, but gocrap does. A negative `limit` makes every function red, and a `sharedLimit` with no `shared` patterns has no effect. gocrap exits with `2` on both.

To read `.arch-go/report.json`, godeps takes the imported package from the quoted text after `imported package` in each line. This depends on the message text of arch-go v2.

### Set the flags and settings

| Flag | Setting in the `godeps` object | Default | Meaning |
| --- | --- | --- | --- |
| `--format F` | `format` | `html` | Output format: `html` or `json` |
| `--arch-go-report <file>` | `archGoReport` | None | Mark the arrows that this arch-go report flags |
| `--crap-report <file>` | `crapReport` | None | Grade functions and boxes from this gocrap report |
| `--dry-report <file>` | `dryReport` | None | Mark functions from this godry report |
| `--editor-url <template>` | `editorURL` | `file://{file}` | Link format for source lines |
| `--title <text>` | `title` | The module path | Title of the map |
| None | `foreign` | None | List of outside module paths to show as ovals |
| None | `omit` | None | List of patterns for packages to leave out |
| None | `order` | None | List of top-level box names, in the order to show them |
| `--skip-tags a,b` | `skipTags`, at the top level | None | Skip files that can only build with these tags |
| `--skip-files a,b` | `skipFiles`, at the top level | None | Skip files that match these patterns |
| `--shared a,b` | `shared`, at the top level | None | These patterns name your shared code |
| Paths on the command line | `paths`, at the top level | The current folder tree | Folders to scan |
| `--help` | None | | Print usage |

This example shows a map with two outside modules and a fixed order:

```json
{
  "shared": ["pkg/**"],
  "gocrap": { "limit": 30, "sharedLimit": 15 },
  "godeps": {
    "title": "Shop",
    "archGoReport": ".arch-go/report.json",
    "crapReport": "crap.json",
    "dryReport": "dry.json",
    "editorURL": "idea://open?file={file}&line={line}",
    "foreign": ["github.com/stripe/stripe-go", "go.uber.org/zap"],
    "omit": ["internal/gen/**"],
    "order": ["web", "billing", "shared"]
  }
}
```

- A flag replaces the setting of the same name. An empty report path means that the report is not set.
- `foreign`: an import at or below an entry draws an arrow to that entry. If entries overlap, the nearest one wins. An entry that nothing imports doesn't show. An entry never takes imports of the module that godeps scans.
- `omit`: each pattern is matched against the folder of a package, with the rules of [`--skip-files`](#skipping-files-by-name). `gen/**` covers the folder `gen` and every folder below it. `gen` alone covers any folder named `gen`, and nothing below it. `.` covers the package in the root folder. godeps doesn't read the files of an omitted package, so they can't cause a parse error.
- `order`: names match the top-level boxes. The box of the shared code is named `shared`.
- `shared`: a package is shared when any of its files matches. To match a folder, write `pkg/**`.

godeps reads `go.mod` from the directory it runs in, and from no other folder. It supports one module. It scans the same files as gocrap, so it follows [the skip rules of godry](#what-godry-skips) and leaves out test files. It leaves out imports of the standard library, imports of outside modules that are not in `foreign`, and imports of code that it didn't scan.

### Use the exit code

| Code | Meaning |
| --- | --- |
| `0` | The output was written |
| `2` | Any error |

godeps has no exit code `1`, because it checks nothing.

These errors exit with `2`:

- A bad flag or setting.
- A Go file that doesn't parse.
- A missing `go.mod`, or a `go.mod` with no `module` line.
- A path outside the run directory.
- A report path that is set but missing, or a report that isn't valid JSON.
- A gocrap report with no `findings` array, or a row with no file or line.
- A godry report with no `findings` array, or a finding with no `left` or `right` file.
- An arch-go report with no `compliance` object.

These warnings go to stderr and don't change the exit code:

| Warning | What to do |
| --- | --- |
| `gocrap rows with no function: N` | Check that the report is from this code. Rows for omitted packages also count |
| `godry report sides with no function: N` | Check that the report is from this code. Sides in omitted packages or test files also count |
| `arch-go lines with no imported package: N` | Check your arch-go version, because the message text may have changed |
| `arch-go breaks with no edge: N` | The map has one or more red arrows too few. The import is in the standard library, in an outside module that is not in `foreign`, or in an omitted package |
| `order name "x" matches no top-level box` | Correct the name in `order` |
| `shared pattern "x" matches no file` | Correct the pattern. To match a folder, write `folder/**` |

### Open the source in your editor

Each link on a card uses `editorURL`. godeps replaces `{file}` with the absolute path of the file and `{line}` with the line number. It encodes each value for a URL.

| Editor | `editorURL` |
| --- | --- |
| JetBrains IDEs | `idea://open?file={file}&line={line}` |
| Zed | `zed://file{file}:{line}` |
| Sublime Text | `subl://open?url=file://{file}&line={line}` |
| TextMate | `txmt://open?url=file://{file}&line={line}` |
| MacVim | `mvim://open?url=file://{file}&line={line}` |

- JetBrains: GoLand also takes `goland://open?file={file}&line={line}`.
- Sublime Text: if a link does nothing, your system has no handler for `subl://`. Install a URL handler for it.
- With no template, the link is `file://{file}`. It opens the file with no line number, in the program that your system chooses.

Neovim has no URL handler. Each item on a card also shows `path/to/file.go:42` as text. Click it to select it, copy it, and open the file with your own commands. If your team wants Neovim links, one way is a small URL handler on your operating system that calls `nvim --server <socket> --remote` and then jumps to the line. That handler is not part of goalie.

### Read the model

`godeps --format json` prints the map as a model, so that a script can read packages, imports, scores, and breaks. The page holds the same model. The same input gives the same bytes. godeps writes to stdout and creates no files.

In the model, a **class** is one Go package, and a **package** is a folder box that groups classes and other boxes. This example has every field. Your output has the same shape:

```json
{
  "version": 1,
  "title": "Shop",
  "module": "example.com/shop",
  "order": ["web"],
  "packages": [
    {
      "id": ":shared/lib/text",
      "name": "text",
      "parent": ":shared/lib",
      "shared": true,
      "packageCount": 1,
      "duplicateCount": 0,
      "grade": "green"
    }
  ],
  "classes": [
    {
      "id": "example.com/lib",
      "name": "example.com/lib",
      "foreign": true,
      "types": [],
      "ops": []
    },
    {
      "id": "example.com/shop/web/cart",
      "name": "cart",
      "path": "web/cart",
      "package": "web/cart",
      "duplicateCount": 1,
      "grade": "red",
      "types": [
        {
          "name": "Order",
          "kind": "struct",
          "file": "web/cart/cart.go",
          "line": 5,
          "url": "file:///repo/web/cart/cart.go",
          "fields": [{ "name": "ID", "type": "string" }]
        },
        {
          "name": "Store",
          "kind": "interface",
          "file": "web/cart/cart.go",
          "line": 10,
          "url": "file:///repo/web/cart/cart.go",
          "methods": [{ "name": "Save", "signature": "(id string) error" }]
        }
      ],
      "ops": [
        {
          "name": "Order.Total",
          "file": "web/cart/cart.go",
          "line": 17,
          "endLine": 33,
          "exported": true,
          "url": "file:///repo/web/cart/cart.go",
          "crap": 6.6,
          "complexity": 5,
          "coverage": 0.6,
          "grade": "yellow",
          "duplicates": [
            {
              "id": "example.com/shop/billing/invoice",
              "name": "Add",
              "file": "billing/invoice/invoice.go",
              "line": 7,
              "url": "file:///repo/billing/invoice/invoice.go",
              "score": 1
            }
          ]
        }
      ]
    }
  ],
  "edges": [
    {
      "from": "example.com/shop/web/cart",
      "to": "example.com/lib",
      "kind": "dependency",
      "violations": [
        {
          "rule": "Packages matching pattern 'example.com/shop/web/cart' should ['not depend on external packages that matches [[example.com/lib]]']",
          "details": ["ShouldNotDependsOn.External rule contains imported package 'example.com/lib/sub'"]
        }
      ]
    }
  ]
}
```

`packages`, `classes`, `edges`, and each `types` and `ops` are always arrays, so a script can read them with no check. godeps leaves out any other field that is empty, unless the table says otherwise. `[]` in a field name means each item of a list.

| Field | Type | Meaning |
| --- | --- | --- |
| `version` | Number | Version of this format. It is `1` |
| `title` | String | `--title`, or else the module path |
| `module` | String | Module path from `go.mod` |
| `order[]` | String | The `order` setting, unchanged |
| `packages[].id` | String | Folder path from the run directory, sorted. The box of the shared code has the id `:shared`, and folders in it start with `:shared/` |
| `packages[].name` | String | Last part of the path. The box of the shared code is named `shared` |
| `packages[].parent` | String | Id of the box above. Left out at the top level |
| `packages[].shared` | Boolean | `true` for shared code. Left out otherwise |
| `packages[].packageCount` | Number | Classes at any depth below this box |
| `packages[].duplicateCount` | Number | Functions at any depth with a duplicate. `0` with a godry report, if there are none. Left out with no report |
| `packages[].grade` | String | `green`, `yellow`, or `red`: the worst function at any depth. Left out with no gocrap report, or with no graded function |
| `classes[].id` | String | Full import path, sorted |
| `classes[].name` | String | The package clause. A foreign class has its full module path. If a folder holds two clauses, the first one that isn't `main` |
| `classes[].path` | String | Folder from the run directory. `.` is the root. Left out for a foreign class |
| `classes[].package` | String | Id of the box that holds the class. Left out at the top level |
| `classes[].foreign` | Boolean | `true` for a `foreign` module, which has no path and empty `types` and `ops` |
| `classes[].duplicateCount` | Number | As on `packages`. A foreign class never has it |
| `classes[].grade` | String | As on `packages` |
| `classes[].types[].name` | String | Type name, with no type parameters |
| `classes[].types[].kind` | String | `struct`, `interface`, or `other` |
| `classes[].types[].file` | String | File from the run directory |
| `classes[].types[].line` | Number | Line of the name |
| `classes[].types[].url` | String | Editor link |
| `classes[].types[].fields[].name` | String | Field name. An embedded field has the bare type name |
| `classes[].types[].fields[].type` | String | Field type as written in the source |
| `classes[].types[].methods[].name` | String | Method name. An embedded interface or a constraint has its source text here |
| `classes[].types[].methods[].signature` | String | Parameters and results. Left out for an embedded interface or a constraint |
| `classes[].ops[].name` | String | `Name` for a function, `Type.Name` for a method, as in gocrap |
| `classes[].ops[].file` | String | File from the run directory |
| `classes[].ops[].line` | Number | Line of the `func` keyword |
| `classes[].ops[].endLine` | Number | Last line of the function |
| `classes[].ops[].exported` | Boolean | Whether the function or method name is exported |
| `classes[].ops[].url` | String | Editor link |
| `classes[].ops[].crap` | Number | gocrap score. Left out if the report has no row for this function |
| `classes[].ops[].complexity` | Number | Cyclomatic complexity. Left out with `crap` |
| `classes[].ops[].coverage` | Number | A fraction from `0` to `1`. Left out with `crap` |
| `classes[].ops[].grade` | String | `green`, `yellow`, or `red`. `red` if the gocrap report has no row for it. Left out with no report, and for an assembly stub |
| `classes[].ops[].duplicates[].id` | String | Class of the other function. Left out when it is not on the map, as with an omitted package or a test file |
| `classes[].ops[].duplicates[].name` | String | Name of the other function |
| `classes[].ops[].duplicates[].file` | String | File of the other function |
| `classes[].ops[].duplicates[].line` | Number | Line of the other function |
| `classes[].ops[].duplicates[].url` | String | Editor link |
| `classes[].ops[].duplicates[].score` | Number | godry score |
| `edges[].from` | String | Id of the class that imports. Sorted by `from`, `to`, and `kind` |
| `edges[].to` | String | Id of the class that is imported |
| `edges[].kind` | String | `dependency` for an import. `composition` for a struct that embeds a type from another class of the module |
| `edges[].violations[].rule` | String | Text of the arch-go rule. A `dependency` edge can have more than one violation. Left out with no break |
| `edges[].violations[].details[]` | String | The arch-go lines for this import |

Edges between boxes are not stored. To find them, follow the class edges up through `package` and `parent`.

### Expect this speed

These numbers come from a laptop with an Apple M4 Max chip. The module has 4,000 files and 48,000 functions, with all three reports.

| Measure | Target | Result |
| --- | --- | --- |
| godeps run | Under 5 seconds | About 1 second, or 1.5 seconds with one package per file |
| Page size | None | 25 to 29 MB |
| Page open in Chrome | Under 2 seconds | About 0.15 seconds |
| Open a box, open a card, show a rule panel | Instant | Within two screen frames, about 33 ms |

## Develop goalie

1. Run `nix develop` to enter a shell with Go and `git`. You can also use your own Go 1.25 or later.
2. Run `go test ./...` to run the tests.
3. Run `nix build` to build the package. The build runs the same tests.

Nix sees only the files that Git tracks. Run `git add` on a new file before you run `nix build`.

### Check the godeps page by hand before a release

The Go tests don't run the page's JavaScript. You must check drill-down, links, and colours in a browser. The page for this check is `internal/godeps/testdata/page/map.html`, which is built from the small module in the same folder. Its links hold a placeholder path, so build a copy with real links:

```sh
go build -o /tmp/godeps ./cmd/godeps
(cd internal/godeps/testdata/page && /tmp/godeps > /tmp/map.html)
```

The second command prints two warnings about the omitted `gen` folder. They are expected.

Open `/tmp/map.html` and follow these steps:

1. Look at the first screen. `shared` comes first in the row of four boxes, because of the `order` setting. `example.com/lib` is an oval. `billing` is red and says `1 duplicated function`. `infra` says `0 duplicated functions`. Four arrows are red and dashed.
2. Hover over the red arrow from `web` to `billing`. A panel shows the rule and the import `web/api` to `billing/tax`, and `billing/invoice` is not in it. Move the pointer away. The panel closes.
3. Click the same arrow and move the pointer away. The panel stays and the arrow is marked. Press Esc. The panel closes, and the level doesn't change.
4. Double-click `billing`. The box opens, `infra` stays at the edge, and the arrow between `invoice` and `tax` is marked `loop`. Hover over the `infra` box at the edge. Its tooltip says `0 duplicated functions`.
5. Reload the page. The same level shows. Press Esc. The first screen shows again.
6. Double-click `web`, and then `cart`. The card lists the types, and the functions with their scores. `Sum` shows `CRAP 6.6`, `Skipped` shows `No gocrap score`, and `Stub` shows no score. The duplicate mark of `Sum` names `Add` with its score, and `Gen` as not on the map.
7. Click the link of `Sum`. The file opens. Click its `web/cart/cart.go:17` text. The whole text is selected.
8. Click a link under Imports on the card. That card opens. Click a level in the breadcrumb. The card closes and that level shows.
9. Turn off the network and reload the page. Nothing changes.

To rebuild `map.html` after you change the page, the module, or the reports, run this command from the repo root. `go test` fails until the file is current:

```sh
go test ./internal/godeps -run FixturePage -update
```

To rebuild the reports after you change the module, run these commands in `internal/godeps/testdata/page`. `crap.json` has no command, because the module holds an import loop on purpose and `go test` can't build it. Edit its line numbers by hand.

```sh
arch-go --json || true
godry --format json --warn-only > dry.json
```

## Credits

gocrap follows [crap4go](https://github.com/unclebob/crap4go), and shares no code with it. Scores can differ from crap4go's scores, because gocrap does not count `default` clauses as branches.

godeps follows the ideas and field names of [uml-viewer](https://github.com/unclebob/uml-viewer), and shares no code with it.

godry follows the method of [dryer](https://github.com/unclebob/dryer) and [dry4clj](https://github.com/unclebob/dry4clj). Scores differ from dryer's scores, because godry parses code with `go/parser` and the two parsers build different trees.

## License

goalie uses the [0BSD license](LICENSE).

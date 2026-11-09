# goalie

goalie stops poor Go code before it merges. It is made for repos where coding agents write code. It has two commands, and both read one settings file, `.goalierc`.

| Command | What it does |
| --- | --- |
| [`goalie check`](#check-your-code) | Checks Go code against a set of rules, and fails a build when a rule fails |
| [`godeps`](#map-your-packages) | Draws the shape of a Go module as a map in one HTML file |

`goalie check` has eleven rules:

| Rule | What it finds |
| --- | --- |
| [`dup`](#find-duplicates-with-dup) | Functions with the same structure, such as a copied function |
| [`crap`](#find-risky-functions-with-crap) | Functions that are complex and poorly tested |
| [`long-function`](#find-long-functions-with-long-function) | Functions with more lines than the limit |
| [`many-arguments`](#find-functions-with-many-parameters) | Functions and methods with more parameters than the limit |
| [`many-dependencies`](#find-functions-with-many-parameters) | Constructors with more parameters than the limit |
| [`complex-condition`](#find-complex-conditions-with-complex-condition) | Functions with a condition that has more `&&` and `\|\|` operators than the limit |
| [`cognitive-complexity`](#find-functions-that-are-hard-to-follow-with-cognitive-complexity) | Functions with more nesting and branches than the limit |
| [`primitive-arguments`](#find-basic-types-passed-down-with-primitive-arguments) | Functions that take a `string`, an `int`, or another basic type and only pass it down to other code in your repo |
| [`long-file`](#find-long-files-with-long-file) | Files with more lines than the limit |
| [`high-mean-complexity`](#find-files-of-complex-functions-with-high-mean-complexity) | Files whose functions have a mean complexity over the limit |
| [`low-cohesion`](#find-types-that-hold-two-jobs-with-low-cohesion) | Struct types whose methods fall into groups that share no field and no call |

goalie uses only the Go standard library.

If you used `godry` or `gocrap`, read [CHANGELOG.md](CHANGELOG.md). Both commands are gone, and it gives the replacement for each command, flag, setting, directive, and environment variable.

## Install goalie

Choose one method.

| Method | Command | What you need |
| --- | --- | --- |
| Go | `go install github.com/n-valchev/goalie/cmd/goalie@latest github.com/n-valchev/goalie/cmd/godeps@latest` | Go 1.25 or later |
| Nix, run once | `nix run github:n-valchev/goalie#goalie -- check --help`, or `nix run github:n-valchev/goalie#godeps -- --help` | Nix with flakes |
| Nix, keep on your `PATH` | `nix profile add github:n-valchev/goalie` | Nix with flakes |

To check the install, run `goalie check --help` and `godeps --help`. Each command prints its flags, settings, and exit codes.

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

## Check your code

To check your branch against `main`, run this command from the repo root:

```sh
goalie check --base main
```

goalie runs your tests to get coverage, and then runs every rule on the functions that you changed since `main`. It prints one line for each result:

```text
billing/total.go:15: fail crap: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 30)
billing/total.go:3: fail dup: duplicates pkg/sum.go:3 (score 1.00, limit 0.82, literals -, anchors 0)
```

Each line gives the file, the line, the mode, the rule, and a message with the value and the limit. The value of `dup` is the score of the pair. The value of `crap` is the score, or the complexity in a partial check. This run exits with `1`, because a result in `fail` mode exists.

| Command | What it checks |
| --- | --- |
| `goalie check --base <rev>` | Every rule that isn't `off`. It runs the tests |
| `goalie check dup --base <rev>` | Only the rules that you name. Separate two names with a space: `goalie check dup crap` |
| `goalie check --partial --base <rev>` | Every rule, with no test run. See [Full and partial checks](#full-and-partial-checks) |
| `goalie check --all` | The whole repository. It needs no revision |

### Every rule has a limit, a mode, and a scope

| Control | Values | Default | Meaning |
| --- | --- | --- | --- |
| Limit | A number | Set by each rule | The value that the code must stay within. A value equal to the limit passes |
| Mode | `off`, `warn`, `fail` | `fail` for `dup` and `crap`. `warn` for the other rules | `off`: no output. `warn`: reports, and never changes the exit code. `fail`: reports, and exits with `1` |
| Scope | `changes`, `all` | `changes` | `changes`: only functions that the change touches. For `long-file` and `high-mean-complexity`, only a file that the change takes over the limit, or a new file over the limit. For `low-cohesion`, only a type whose declaration or methods the change touches. `all`: every function, file, or type |

- Only `"mode": "off"` turns a rule off. A limit of `0` is a normal limit.
- A new rule defaults to `warn`, so that adopting it can't block work. Set `"mode": "fail"` when you have checked its limit on your repository.
- A rule that you name runs even if it is `off`, and then it reports as `warn`. Use this to try a limit on real code without failing a build.
- A rule with scope `changes` needs a revision. goalie takes it from `--base`, or else from `base` in `.goalierc`. With neither, goalie exits with `2` and prints `goalie: needs a revision: use --base <rev>, or set base in .goalierc`. goalie has no default revision, because a default of `HEAD` would see no changes in CI, and a forgotten flag would pass every build.
- `--all` gives every rule scope `all`, and it needs no revision. Each rule still follows its mode, so an old repository can exit with `1`. Use `--all` to analyse a repository, not in CI.

The changes include commits since the merge base, uncommitted changes, and untracked files. A function counts as changed when a changed line falls inside it. A function that you moved counts as changed.

### Full and partial checks

| Check | Command | Runs tests | Every rule except `crap` | `crap` |
| --- | --- | --- | --- | --- |
| Full | `goalie check` | Yes | The exact result | The exact score |
| Partial | `goalie check --partial` | No | The exact result | A result from complexity alone |

CI runs the full check. A partial check runs no tests, so it is fast enough for an agent to run while it works. See [Steer coding agents](#steer-coding-agents).

In a partial check, `crap` puts each function in one of three cases. Here `c` is the complexity of the function and `L` is its limit:

| Case | Condition | Result |
| --- | --- | --- |
| Safe | `c² + c ≤ L` | No output, because the function passes even with no coverage |
| Needs coverage | Between the other two cases | A `warn` result, in any mode, with the coverage that the function needs. Only a test run can tell whether it has that coverage |
| Can't pass | `c > L` | A result that follows the mode of `crap`. No coverage can help, so split the function |

At the default limit of 30, complexity 5 is safe, complexity 10 needs 42% coverage, complexity 20 needs 71%, and complexity 31 can't pass. A result for a function that needs coverage looks like this:

```text
billing/total.go:15: warn crap: Decide needs at least 13% coverage (complexity 6, limit 30)
```

### Read the results

- `fail` results come first, then `warn` results. Within each mode, the highest value divided by its limit comes first. A limit of `0` counts as `1` for this order.
- stdout holds only results, so a clean run prints nothing there. Notes go to stderr: the output of your tests, and a line such as `goalie: dup: No new functions found.` when `dup` or `crap` has nothing to check. The other rules write no such line.
- `--format json` prints an object with a `findings` array, which is `[]` for a clean run. Each entry has `rule`, `mode`, `value`, and `limit`, and the fields of its rule:

| Rule | Fields |
| --- | --- |
| `dup` | `score`, `literals` (a number, or `null` for `-`), `anchors`, `left`, and `right`. With `shared` patterns, also `case`. `left` and `right` each have `file`, `function`, `startLine`, `endLine`, `nodes`, and `new` |
| `crap`, full check | `file`, `line`, `function`, `score`, `complexity`, and `coverage` (a fraction from `0` to `1`) |
| `crap`, partial check | `file`, `line`, `function`, `complexity`, `canPass`, and `neededCoverage` (a fraction, left out when `canPass` is `false`) |
| `long-function`, `many-arguments`, `many-dependencies`, `complex-condition`, `cognitive-complexity`, and `primitive-arguments` | `file`, `line`, and `function`. For `complex-condition`, `line` is the line of the worst condition. For `primitive-arguments`, `parameters` lists each reported parameter with its `name` and `type` |
| `long-file` | `file` and `line`. `line` is always `1` |
| `high-mean-complexity` | `file`, `line`, and `functions`, the number of functions that the mean covers. `line` is always `1` |
| `low-cohesion` | `file`, `line`, `type`, and `groups`, a list of lists of method names. `line` is the line of the type declaration |

### Use the exit code

| Code | Meaning |
| --- | --- |
| `0` | No `fail` result. `warn` results never change the exit code |
| `1` | At least one `fail` result |
| `2` | Any error |

These are some of the errors: an invalid flag, an unknown rule name, a missing revision, an unknown revision, `--base` outside a Git repository, an invalid `.goalierc`, a Go file that doesn't parse, a `//go:build` line that doesn't parse, and a failed coverage command.

If your tests fail, goalie still reports the other rules. It skips `crap`, prints the test failure on stderr, and exits with `2`. You see every other problem in the same run.

### Change one rule for one run

These flags work only when you name exactly one rule. They replace `.goalierc` for that run.

| Flag | Meaning |
| --- | --- |
| `--limit <n>` | The limit of the rule. For `dup`, this is the threshold, from `0` to `1` |
| `--shared-limit <n>` | The `sharedLimit` of `crap`. See [Set the limits of crap](#set-the-limits-of-crap) |
| `--warn-only` | Report the rule in `warn` mode, so that its results never exit with `1`. An error still exits with `2` |

For example, to list every function with its score:

```sh
goalie check crap --all --limit 0 --warn-only
```

## Set the rules in `.goalierc`

To keep your settings in the repo, write them in a `.goalierc` file. The file holds one JSON object:

```json
{
  "base": "origin/main",
  "paths": ["products/billing", "products/search", "pkg"],
  "skipFiles": ["packages.go", "internal/generated/**"],
  "skipTags": ["integration"],
  "shared": ["pkg/**"],
  "rules": {
    "dup": { "mode": "fail", "scope": "changes", "threshold": 0.82, "weakAnchors": 1 },
    "crap": { "mode": "warn", "limit": 30, "sharedLimit": 15, "coverCommand": "make coverage" },
    "long-function": { "mode": "fail", "limit": 60 },
    "many-arguments": { "limit": 5 }
  }
}
```

The settings at the top level describe the repo, and every rule uses them:

| Setting | Type | Meaning |
| --- | --- | --- |
| `base` | String | The revision for rules with scope `changes`. `--base` replaces it |
| `paths` | List of strings | Folders to check. The default is every folder below the directory goalie runs in |
| `skipTags` | List of strings | Skip files that can only build with these tags |
| `skipFiles` | List of strings | Skip files that match these patterns. See [Skip files by name](#skip-files-by-name) |
| `shared` | List of strings | Patterns that name your shared code. See [Name your shared code](#name-your-shared-code) |

Each rule has its own object in `rules`:

| Rule | Settings |
| --- | --- |
| Every rule | `mode` and `scope`. See [Every rule has a limit, a mode, and a scope](#every-rule-has-a-limit-a-mode-and-a-scope) |
| `dup` | `threshold`, `minLines`, `minNodes`, `weakAnchors`, `weakLiterals`, and `includeTests`. See [Set the limits of dup](#set-the-limits-of-dup) |
| `crap` | `limit`, `sharedLimit`, `coverCommand`, and `coverProfiles`. See [Set the limits of crap](#set-the-limits-of-crap) |
| `long-function` | `limit`. See [Find long functions with long-function](#find-long-functions-with-long-function) |
| `many-arguments` and `many-dependencies` | `limit`. See [Find functions with many parameters](#find-functions-with-many-parameters) |
| `complex-condition` | `limit`. See [Find complex conditions with complex-condition](#find-complex-conditions-with-complex-condition) |
| `cognitive-complexity` | `limit`. See [Find functions that are hard to follow with cognitive-complexity](#find-functions-that-are-hard-to-follow-with-cognitive-complexity) |
| `primitive-arguments` | `limit`. See [Find basic types passed down with primitive-arguments](#find-basic-types-passed-down-with-primitive-arguments) |
| `long-file` | `limit`. See [Find long files with long-file](#find-long-files-with-long-file) |
| `high-mean-complexity` | `limit`. See [Find files of complex functions with high-mean-complexity](#find-files-of-complex-functions-with-high-mean-complexity) |
| `low-cohesion` | `limit`. See [Find types that hold two jobs with low-cohesion](#find-types-that-hold-two-jobs-with-low-cohesion) |

- Every setting is optional. A missing setting uses its default, and a missing file is not an error.
- goalie reads `.goalierc` from the directory it runs in, and from no other folder. Paths and patterns start from that directory. Run goalie from the repo root in CI.
- Invalid JSON, an unknown setting, or a bad value exits with `2`. A `godry` or `gocrap` section is an error that names the new place, for example `goalie: .goalierc: move "godry" settings to "rules.dup"`.
- `--format` and `--warn-only` are flags only. `mode` does the work of the old `warnOnly` setting.
- If you set `base`, every run uses it for rules with scope `changes`. `--base` replaces it for one run, and `--all` turns it off for one run.
- JSON has no comments. To record why a file is skipped, put the pattern in `.goalieignore`.

### Require approval for changes to the settings

If agents work in your repo, require human approval for changes to `.goalierc` and `.goalieignore`, for example with `CODEOWNERS`. Otherwise, an agent can pass the check by weakening the settings.

- A mode change has the same effect as a limit change. `"mode": "warn"` lets every result pass, as a very high limit does, and `"mode": "off"` hides the results.
- `coverCommand` can run any command, so an agent can also use it to raise its scores.
- A `//goalie:ignore` directive is in the code, not in the settings. Check its reason in code review.

## Choose the files to check

goalie checks the Go files below the directory it runs in, or in `paths`. Paths limit every rule: `dup` never compares a function outside them.

goalie skips these files and folders, even inside `paths`:

- The folders `vendor`, `testdata`, and `node_modules`, and folders with names that start with `.` or `_`.
- `*_test.go` files. `dup` reads them only if you set `includeTests`, and `crap` never scores them.
- Files with the header `Code generated ... DO NOT EDIT.`
- Files with an `ignore` build constraint.
- Files that can only build with a tag in `skipTags`.
- Files that match a pattern in `skipFiles` or in `.goalieignore`.

Other build constraints and `GOOS` or `GOARCH` file suffixes have no effect. The same commit gives the same result on every machine.

### Skip files by name

Use `skipFiles` for files that repeat on purpose, such as dependency wiring in every package.

| Pattern | Matches |
| --- | --- |
| `packages.go` | `packages.go` in any folder |
| `wire_*.go` | Any file name that starts with `wire_`, in any folder |
| `internal/users/packages.go` | That one file, from the directory goalie runs in |
| `internal/*/packages.go` | `packages.go` one folder below `internal`, because `*` does not cross folders |
| `internal/generated/**` | Everything below that folder, at any depth |
| `**/generated/**` | Everything below any folder named `generated` |
| `internal/**/packages.go` | `packages.go` anywhere below `internal` |

A skipped file is not read, so it can't cause a parse error. A bad pattern exits with `2`.

To record why you skip a file, write the patterns in a `.goalieignore` file instead:

```text
# Dependency wiring repeats on purpose.
packages.go
wire_*.go
internal/legacy/client.go
```

- goalie reads `.goalieignore` from the directory it runs in, and from no other folder.
- Write one pattern per line. The patterns follow the table above. Blank lines and lines that start with `#` are ignored.
- `.goalieignore` and `skipFiles` both apply.
- A missing file is not an error. A bad pattern exits with `2` and names the line.
- This is not full `.gitignore` syntax. `!`, a trailing `/`, and `**` inside a longer name such as `a**b` exit with `2`. To skip a folder, write `folder/**`.

### Name your shared code

In a monorepo, the right fix for a duplicate depends on where the two functions are. To add this to each `dup` result, list your shared code in `shared`:

```json
{ "shared": ["pkg/**"] }
```

```text
billing/total.go:3: fail dup: duplicates pkg/sum.go:3 (score 1.00, limit 0.82, literals -, anchors 0, case copies-shared)
```

| Case | Where the functions are | Usual fix |
| --- | --- | --- |
| `copies-shared` | One in shared code, one in a product | Delete the copy and use the shared function |
| `across-products` | In two different products | Decide whether to move the code to shared code |
| `same-product` | Both in one product | Fix it inside that product |
| `within-shared` | Both in shared code | Merge the two functions |

- `shared` uses the same patterns as `skipFiles`. To match a folder, write `pkg/**`, because `pkg` alone matches a file name.
- A product is the first folder in the path from the directory goalie runs in, unless a shared pattern matches the file. goalie has no list of products, so every other first folder counts as one, including folders such as `tools`. To leave a folder out, don't list it in `paths`, or skip it.
- A file directly in the directory goalie runs in, or above it, belongs to no product. A result with such a file has no case, unless both functions are in shared code.
- `shared` doesn't change which pairs `dup` reports, or the exit code.
- `crap` holds shared code to `sharedLimit`. See [Set the limits of crap](#set-the-limits-of-crap).
- If a pattern matches no file, goalie prints a warning on stderr, such as `goalie: shared pattern "pkgs/**" matches no file; to match a folder, write folder/**`. The run continues.

## Excuse a function from a rule

To excuse one function from named rules, write a `//goalie:ignore` directive in its doc comment. A directive is a comment line that goalie reads. Give the rule names and a reason:

```go
//goalie:ignore dup The two parsers must stay independent.
func parseHeader(line string) (Header, error) {
```

```go
// Decide maps each case to its code.
//goalie:ignore crap,dup The table of cases needs every branch.
func Decide(kind string) int {
```

- The rules that the directive names excuse the function. Every other rule still checks it.
- To name two rules, separate them with a comma and no space. You can also write one directive for each rule.
- A `dup` directive also excuses every pair with the function in it, on either side.
- An unknown rule name or a missing reason exits with `2`, for example `goalie: dup: billing/skip.go:16: //goalie:ignore needs a reason`.
- `//godry:ignore` and `//gocrap:ignore` have no effect. Replace them with `//goalie:ignore dup` and `//goalie:ignore crap`.
- For `low-cohesion`, write the directive directly above the type declaration, not above a method: `//goalie:ignore low-cohesion <reason>`.
- A directive can't excuse a whole file, and `long-file` and `high-mean-complexity` have no directive. To skip a file, use `skipFiles` or `.goalieignore`. See [Choose the files to check](#choose-the-files-to-check).

## Find duplicates with `dup`

`dup` finds Go functions with the same structure. It compares functions across the whole repository, not one package at a time.

A renamed copy still matches. `dup` ignores the names of local variables, the names of fields, and the values of literals. It keeps the names of called functions and the operators, so functions that do different work don't match.

| Scope | Compares | Reports |
| --- | --- | --- |
| `changes` | Each changed function with every checked function | Only pairs with at least one changed function. The changed function comes first |
| `all` | Every function with every other function | All pairs at or above the threshold |

With scope `changes`, pairs of old functions don't appear, so existing duplication doesn't block unrelated work.

`dup` also skips pairs in one package with the same function name and the same receiver type. These are usually platform variants.

### Read a `dup` result

```text
billing/total.go:3: fail dup: duplicates pkg/sum.go:3 (score 1.00, limit 0.82, literals -, anchors 0)
```

The score runs from `0` to `1`, and `1.00` means the two functions have the same structure. The other two numbers describe the match. They don't decide whether `dup` reports a pair, unless you set `weakAnchors`:

| Number | Meaning | How to read it |
| --- | --- | --- |
| `literals` | Literals both functions share, divided by all literals in either function. `-` means neither has one to compare. `0`, `1`, `""`, `nil`, `true`, and `false` don't count. | High: a pasted copy. Low: the same code with different data. |
| `anchors` | Different called names that both functions use. Operators don't count. A type conversion such as `string(x)` looks like a call, so it counts. | `0` with low `literals`: only the shape of the code matches, as with two `switch` mappings. |

### Set the limits of `dup`

| Setting in `rules.dup` | Type | Default | Meaning |
| --- | --- | --- | --- |
| `threshold` | Number from `0` to `1` | `0.82` | The limit: the minimum score that `dup` reports. `--limit` replaces it |
| `minLines` | Whole number | `9` | Minimum lines in a function |
| `minNodes` | Whole number | `20` | Minimum nodes in the normalized function |
| `weakAnchors` | Whole number | `0` | Drop results with fewer anchors than this and low literals. `0` drops nothing |
| `weakLiterals` | Number from `0` to `1` | `0.5` | Literal score below which a result counts as low |
| `includeTests` | `true` or `false` | `false` | Also compare functions in `*_test.go` files |

### Drop weak results

Two functions can match on shape alone, such as two `switch` statements that map values to strings. To drop these results, set:

```json
{ "rules": { "dup": { "weakAnchors": 1, "weakLiterals": 0.5 } } }
```

`dup` drops a result only when both numbers are low:

| Anchors | Literals | Result | Why |
| --- | --- | --- | --- |
| Below `weakAnchors` | Below `weakLiterals` | Dropped | Only the shape matches |
| At or above the limit | Any | Kept | The functions call the same code |
| Any | At or above the limit | Kept | The functions hold the same data |
| Any | `-` | Kept | Nothing shows that the data differs |

- A dropped result doesn't count, so a run that drops every result exits with `0`.
- goalie prints the count on stderr, for example `goalie: 12 weak findings dropped`.
- The rule applies to both scopes. It runs after comparison, so it doesn't change which pairs match.

## Find risky functions with `crap`

`crap` scores each function by its complexity and its test coverage. A high score means the function is risky to change. It uses the CRAP (Change Risk Anti-Pattern) formula, where CC is cyclomatic complexity:

```text
score = CC² × (1 - coverage)³ + CC
```

For example, a function with a complexity of 6 scores 42 with no coverage, and 10.5 with 50% coverage. Full coverage leaves a score equal to the complexity, so a very complex function fails even if it is fully tested.

```text
billing/total.go:15: fail crap: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 30)
```

With scope `changes`, `crap` scores only the changed functions. It still runs all your tests, because any test can cover a changed function. If no function changed, it runs nothing.

### Set the limits of `crap`

| Setting in `rules.crap` | Type | Default | Meaning |
| --- | --- | --- | --- |
| `limit` | Number | `30` | The highest score that passes. `--limit` replaces it |
| `sharedLimit` | Number | The value of `limit` | The limit for files that match `shared`. `--shared-limit` replaces it |
| `coverCommand` | String | See [Measure all your test suites](#measure-all-your-test-suites) | The command that writes coverage profiles |
| `coverProfiles` | List of strings, each a file or a glob | None | Coverage profiles to read in place of a command |

- Every product depends on shared code, so you can hold it to a stricter limit with `sharedLimit`. `sharedLimit` without `shared` is an error.
- To list every function, run `goalie check crap --all --limit 0 --warn-only`. If you set `shared`, also add `--shared-limit 0`.

### Measure all your test suites

`crap` runs one command to gather coverage. With no settings, the command is:

```sh
go test -coverpkg=./... -coverprofile="$GOALIE_COVER_DIR/coverage.out" ./...
```

If your repo has more suites, set `coverCommand`. goalie follows these steps:

1. It creates an empty temporary folder and sets `GOALIE_COVER_DIR` to that path.
2. It runs your command with `sh -c`, from the directory it runs in.
3. It reads each file in that folder as a text coverage profile. It ignores subfolders.
4. It merges the profiles. A code block counts as covered if any profile covers it.

This example runs unit, integration, and end-to-end (e2e) tests:

```sh
set -e
go test -coverpkg=./... -coverprofile="$GOALIE_COVER_DIR/unit.out" ./...
go test -tags integration -coverpkg=./... -coverprofile="$GOALIE_COVER_DIR/integration.out" ./...

mkdir "$GOALIE_COVER_DIR/raw"
go build -cover -coverpkg=./... -o "$GOALIE_COVER_DIR/raw/app" ./cmd/app
GOCOVERDIR="$GOALIE_COVER_DIR/raw" ./e2e/run.sh "$GOALIE_COVER_DIR/raw/app"
go tool covdata textfmt -i="$GOALIE_COVER_DIR/raw" -o="$GOALIE_COVER_DIR/e2e.out"
```

- Keep `-coverpkg=./...`. Without it, Go counts coverage only in the package that holds the test, and code reached from other packages scores as untested.
- An e2e binary must exit normally, because a killed process writes no coverage. In a container, mount the `GOCOVERDIR` folder.
- If the command fails, goalie skips `crap` and exits with `2`. A suite that stopped early would give falsely high scores. Use `set -e` so that a failed suite fails the command.
- The command's output goes to stderr, so stdout stays the report.
- A function in no profile has 0% coverage. This includes files that no test run builds, such as files for another platform. To leave them out, use `skipFiles` or `skipTags`.

If CI has already run the tests, list the files in `coverProfiles`. goalie then runs no command. An entry may be a glob, such as `"coverage/*.out"`. A glob matches files only, and a glob that matches no file is an error. Setting both `coverCommand` and `coverProfiles` is an error.

goalie supports one Go module, and it reads `go.mod` from the directory it runs in, or above it. A profile that holds a file outside that module is an error. Limit `paths` to one module if your repo holds several.

## Find long functions with `long-function`

`long-function` reports a function with more lines than its limit. A long function is hard for a person or an agent to read and to change. The lines run from the `func` keyword to the closing brace. Blank lines and comments count.

```text
billing/parse.go:12: warn long-function: Parse has 92 lines (limit 80)
```

| Setting in `rules.long-function` | Type | Default | Meaning |
| --- | --- | --- | --- |
| `limit` | Number | `80` | The most lines that pass. `--limit` replaces it |

- The default mode is `warn`, so the rule never changes the exit code until you set `"mode": "fail"`.
- It skips `*_test.go` files, and functions with no body, such as assembly stubs.
- It gives the same result in a full and a partial check, and it runs no tests.

## Find functions with many parameters

`many-arguments` and `many-dependencies` report a function with more parameters than its limit. Each extra parameter is one more thing that every caller must supply, and one more thing a test must set up.

```text
billing/charge.go:20: warn many-arguments: Charge has 6 parameters (limit 4)
billing/service.go:8: warn many-dependencies: NewService has 7 parameters (limit 5)
```

A function uses one of the two rules, never both:

| Function | Rule | Default limit |
| --- | --- | --- |
| A constructor: a function, not a method, named `New` or `New` and a capital letter, such as `NewServer` | `many-dependencies` | `5` |
| Every other function and method, including `Newline` and a method named `NewServer` | `many-arguments` | `4` |

A constructor gets a higher limit, because it takes the dependencies of what it builds.

The count works like this:

| Parameter list | Count |
| --- | --- |
| `a, b int` | 2: each name counts |
| `int, string` | 2: an unnamed parameter counts as 1 |
| `opts ...Option` | 1: a variadic parameter counts as 1 |
| The receiver of a method | 0 |

| Setting in `rules.many-arguments` or `rules.many-dependencies` | Type | Default | Meaning |
| --- | --- | --- | --- |
| `limit` | Number | `4` or `5` | The most parameters that pass. `--limit` replaces it |

- The default mode is `warn`, so the rules never change the exit code until you set `"mode": "fail"`.
- `//goalie:ignore many-arguments` doesn't hide a `many-dependencies` result. Name the rule that reports the function.
- They skip `*_test.go` files. They give the same result in a full and a partial check, and they run no tests.

## Find complex conditions with `complex-condition`

`complex-condition` reports a function with a condition that has more `&&` and `||` operators than its limit. A reader must keep every part of such a condition in mind at once.

```text
billing/charge.go:31: warn complex-condition: Charge has a condition with 5 operators (limit 3)
```

A condition is the expression of an `if`, of a `for`, or of a `case`:

| Code | Counts |
| --- | --- |
| `if a && b \|\| c {` | Yes: 2 operators |
| `for i < n && !done {` | Yes: 1 operator |
| `case a && b, c && d:` | Yes: 2 operators. The expressions of one `case` are one condition |
| `if ok() && func() bool { return a && b }() {` | Yes: 2 operators. An operator in a closure in the condition counts |
| `return a && b && c && d && e` | No |
| `ready := a && b && c && d && e` | No |
| `if x&1 \| x&2 != 0 {` | No: `&` and `\|` work on bits, so they don't count |
| A `case` of a `select` | No: it has no boolean expression |

A `return` and an assignment don't count, because a boolean expression with a name is often the fix. To fix a result, give parts of the condition names, such as `ready := a && b`, or move the condition into a function.

A function reports once, with the operators of its worst condition and the line of that condition. If two conditions have the same count, the first one reports.

| Setting in `rules.complex-condition` | Type | Default | Meaning |
| --- | --- | --- | --- |
| `limit` | Number | `3` | The most operators in one condition that pass. `--limit` replaces it |

- The default mode is `warn`, so the rule never changes the exit code until you set `"mode": "fail"`.
- It skips `*_test.go` files. It gives the same result in a full and a partial check, and it runs no tests.

## Find functions that are hard to follow with `cognitive-complexity`

`cognitive-complexity` reports a function with a cognitive complexity over its limit. Cognitive complexity is a score for how hard code is to follow. It comes from SonarSource. The tool `gocognit` measures it for Go. The result points at the first line of the function.

```text
billing/charge.go:12: warn cognitive-complexity: Charge has cognitive complexity 18 (limit 15)
```

Each branch adds to the score, and a branch inside another block adds more:

| Code | Adds |
| --- | --- |
| `if`, `for`, `range`, `switch`, or `select` | 1, plus 1 for each block that it is inside |
| Each `case` of a `switch` or `select` | 0: the `switch` or `select` counts once |
| `else` or `else if` | 1, however deep it is |
| A closure (`func() { ... }` in a function) | 0, but the code inside it counts as 1 block deeper |
| `&&` and `\|\|` | 0: `complex-condition` reports them |
| A call of the function to itself | 0 |

So nesting adds more than branches side by side. Three `if` statements side by side score 3. Three nested `if` statements score 1 + 2 + 3 = 6, and a fourth nested `if` makes it 10.

### A Go guard adds 0

A guard is an `if` that checks for an error or a failed lookup and exits at once. A guard adds 0, so that ten `if err != nil { return err }` checks in a row score 0. An `if` is a guard when all of these are true:

- It has no `else`.
- Its condition is `err != nil` or `!ok`, including the form `if err := f(); err != nil`. A name that ends in `Err`, such as `readErr != nil`, also counts.
- Its body holds no other block, and its last statement is `return`, `continue`, `break`, or `panic(...)`.

| Code | Adds |
| --- | --- |
| `if err != nil { return fmt.Errorf("read: %w", err) }` | 0 |
| `if !ok { continue }` | 0 |
| `if err := func() error { ... }(); err != nil { return err }` | 0 for the guard, but each branch in the closure adds |
| `if err != nil { log.Print(err) }` | As any `if`: the body doesn't exit |
| `if err != nil { if retry { ... }; return err }` | As any `if`, and the inner `if` adds too |
| `if err != nil { return err } else { ... }` | As any `if`, and 1 for the `else` |
| `if failure != nil { return failure }` | As any `if`: goalie goes by the name, because it reads no types |

To fix a result, return early in place of a nested `if`, or move a nested block into its own function.

| Setting in `rules.cognitive-complexity` | Type | Default | Meaning |
| --- | --- | --- | --- |
| `limit` | Number | `15` | The highest score that passes. `--limit` replaces it |

- The default mode is `warn`, so the rule never changes the exit code until you set `"mode": "fail"`.
- It skips `*_test.go` files. It gives the same result in a full and a partial check, and it runs no tests.

## Find basic types passed down with `primitive-arguments`

`primitive-arguments` reports a function with a parameter of a basic type that the function only passes down to other code in your repo. A chain is a row of functions where each one passes the same value down to the next. When the type of that value changes later, every function in the chain must change too. With a defined type, such as `type UserID string`, a function that only passes the value down doesn't change. The result points at the first line of the function and names each parameter and its type:

```text
users/service.go:14: warn primitive-arguments: Service.Get passes tenantID down as string, id down as string (2, limit 1)
```

```go
// Reported: tenantID and id are only passed down.
func (s *Service) Get(ctx context.Context, tenantID, id string) (User, error) {
    return s.repo.Find(ctx, tenantID, id)
}

// Not reported: name is used here.
func greet(name string) string {
    return "Hello, " + strings.TrimSpace(name)
}
```

A basic type is a built-in type such as `string`, `int`, `float64`, `bool`, or `byte`. `error`, `any`, slices, maps, pointers, and function types are not basic types.

A parameter reports only when the function passes it down every time that it appears. Passing it down means giving it, as it is, as an argument to a call into your repo's code. Every other appearance is a use, and a parameter with a use doesn't report:

| Code | Reports |
| --- | --- |
| `s.repo.Find(ctx, id)` or `find(id)` | Yes: the call goes to your repo's code |
| `UserID(id)` | Yes: goalie reads no types, so it can't tell a conversion to your own type from a call to a function in your repo |
| `os.ReadFile(path)` | No: this is a use, because there the value leaves your repo's code and the basic type is required |
| `len(id)` | No: a call to a built-in function is a use |
| `int64(n)`, `[]byte(s)`, or `any(s)` | No: a conversion to a type that Go declares is a use |
| `id == ""`, `"user " + id`, or `&id` | No: the function uses the value |
| Never used | No: `go vet` and other linters report unused parameters |

Each function reports on its own, so a chain of three functions that pass `tenantID` and `id` down gives three results. The value is the number of reported parameters. With the default limit of `1`, a function with one such parameter doesn't report, so a function that passes down only `id` gives no result. On a monorepo of 11,678 functions, a limit of `0` reported 1 function in 9. An agent can't fix such a result in one edit, because the fix is a new type across a whole chain. To report one parameter too, set the limit to `0`.

goalie reads no types, so the rule has these gaps:

- A method whose signature an interface forces reports too. Excuse it with `//goalie:ignore primitive-arguments <reason>`.
- The rule can't see an alias defined elsewhere, such as `type UserID = string`, so a parameter of type `UserID` doesn't report. An alias gives none of the benefit of a defined type.
- goalie knows an imported package by its name. For an import with no name, it takes the name from the path: `gopkg.in/yaml.v3` is `yaml`, `math/rand/v2` is `rand`, and `github.com/mattn/go-sqlite3` is `sqlite3`. When the package has another name, give the import that name, or goalie counts the call as a call into your repo.
- A call into a package that you import with `.`, such as `TrimSpace(name)` after `import . "strings"`, counts as a call into your repo.
- goalie goes by the parameter's name. A variable of the same name in a closure counts as a use.

To fix a result, define a type for the value, such as `type UserID string`, and use it in each function of the chain.

| Setting in `rules.primitive-arguments` | Type | Default | Meaning |
| --- | --- | --- | --- |
| `limit` | Number | `1` | The highest number of reported parameters that passes. `--limit` replaces it |

- The default mode is `warn`, so the rule never changes the exit code until you set `"mode": "fail"`.
- It skips `*_test.go` files. It gives the same result in a full and a partial check, and it runs no tests.

## Find long files with `long-file`

`long-file` reports a file with more lines than its limit. A long file is hard for a person or an agent to read and to search. Every line of the file counts, blank lines and comments too. The result points at line 1, because the whole file is the problem:

```text
billing/parse.go:1: warn long-file: billing/parse.go has 1204 lines (limit 1000)
```

With scope `changes`, a file reports only when the change takes it over the limit. An old long file gives no result, so that it doesn't report on every edit:

| The file at the merge base | The file now | Result |
| --- | --- | --- |
| 1,000 lines or fewer | More than 1,000 lines | Reports |
| More than 1,000 lines | More than 1,000 lines | No result, even if the change added lines |
| Not there, because the file is new, moved, or renamed | More than 1,000 lines | Reports |

With scope `all` or `--all`, every file over the limit reports.

- goalie reads the changed files at the merge base with one Git call. If that call fails, `long-file` reports nothing and prints the reason on stderr. It never fails the check for this.
- A moved or renamed file counts as new, because goalie doesn't follow renames.
- It has no directive, so `//goalie:ignore long-file` exits with `2` as an unknown rule. To leave out a file, use `skipFiles` or `.goalieignore`.

| Setting in `rules.long-file` | Type | Default | Meaning |
| --- | --- | --- | --- |
| `limit` | Number | `1000` | The most lines that pass. `--limit` replaces it |

- The default mode is `warn`, so the rule never changes the exit code until you set `"mode": "fail"`.
- It skips `*_test.go` files. It gives the same result in a full and a partial check, and it runs no tests.

## Find files of complex functions with `high-mean-complexity`

`high-mean-complexity` reports a file whose functions have a mean complexity over its limit. A file of many complex functions is hard to change, even when no single function scores over the `crap` limit. Complexity is the cyclomatic complexity that `crap` uses: 1, plus 1 for each `if`, `for`, `range`, `&&`, and `||`, and for each `case` of a `switch` or `select` other than `default`. The result points at line 1 and names the number of functions, so that you can judge the mean:

```text
billing/parse.go:1: warn high-mean-complexity: billing/parse.go has mean complexity 7.4 over 12 functions (limit 7)
```

- Every function and method in the file counts. A closure counts as part of the function that holds it.
- A function with no body, such as one written in assembly, doesn't count.
- A file with no functions gives no result.
- Scope `changes` works as it does for `long-file`: a file reports only when the change takes it over the limit, or when it is new. A file with no functions at the merge base counts as a new file, so it reports when the change takes its mean over the limit. If the file doesn't parse at the merge base, it gives no result.
- It has no directive. `//goalie:ignore crap` on a function doesn't change the mean, and `//goalie:ignore high-mean-complexity` exits with `2` as an unknown rule.

| Setting in `rules.high-mean-complexity` | Type | Default | Meaning |
| --- | --- | --- | --- |
| `limit` | Number | `7` | The highest mean that passes. `--limit` replaces it |

- The default mode is `warn`, so the rule never changes the exit code until you set `"mode": "fail"`.
- It skips `*_test.go` files. It gives the same result in a full and a partial check, and it runs no tests.

## Find types that hold two jobs with `low-cohesion`

`low-cohesion` reports a struct type whose methods fall into more groups than its limit. It follows LCOM4 (lack of cohesion of methods, version 4). Two methods are in one group when one of these is true:

- Both use the same field of the receiver, such as `s.items`.
- One calls the other on the receiver, such as `s.load()`.
- A third method joins them, because it is in a group with each of them.

A type with 2 groups holds two jobs, and could split into two types. In this example, `Get` and `Put` share `s.items`, and `Ping` and `Close` share `s.conn`. No field or call joins the two groups:

```go
type Store struct {
	items map[string]Item
	conn  *sql.DB
}

func (s *Store) Get(k string) Item    { return s.items[k] }
func (s *Store) Put(k string, v Item) { s.items[k] = v }
func (s *Store) Ping() error          { return s.conn.Ping() }
func (s *Store) Close() error         { return s.conn.Close() }
```

The result points at the type declaration, and names the methods of each group, so that you can see where to split the type. The methods in each group are in name order, and the groups are in the order of their first method:

```text
store.go:12: warn low-cohesion: Store has 2 groups of methods: Close, Ping; Get, Put (limit 1)
```

- The methods of the type count from every file of its package, with a pointer or a value receiver. Methods in `*_test.go` files don't count.
- A method that uses no field and calls no method is left out, such as `func (s *Store) Name() string { return "store" }`. A method that another method calls stays in.
- An accessor is left out. An accessor is a method whose body only returns a receiver field, a field of that field, or the address of either, such as `return s.conn`, `return s.conn.addr`, or `return &s.conn`. It may have parameters. Without this, a type with one accessor for each field, such as a config type, reports one group for each field.
- A call to an accessor, such as `s.Conn()`, counts as a use of the field that the accessor returns.
- Any other body is not an accessor. Examples are `return s.items[k]`, `return fmt.Sprintf("%s", e.Msg)`, `return s.Ping` (a method value), and a setter. So an error type with `Error` and `Unwrap` gives no result, because only `Error` is left.
- A method with variants in two files, such as one for each platform, is an accessor only when every variant returns the same field.
- A method that passes the whole receiver to a call, such as `save(s)` or `save(*s)`, joins every group, because the function it calls can use any field.
- A field use inside a closure in a method counts.
- A selector on the receiver that names no field and no method, such as `s.Lock()` from an embedded `sync.Mutex`, counts as a use of one shared field. goalie reads no types, so it can't tell which embedded field holds `Lock`.
- A copy of the receiver, such as `c := s`, is not followed, so a field use through `c` doesn't count.
- The rule skips interfaces, defined types that aren't structs, such as `type Kind int`, and a type with fewer than 2 methods left.
- Scope `changes` reports a type when the change touches its declaration or one of its methods, in any file of the package.
- To excuse a type, write `//goalie:ignore low-cohesion <reason>` directly above the type declaration. A `//goalie:ignore low-cohesion` above a method has no effect: the method still counts.

| Setting in `rules.low-cohesion` | Type | Default | Meaning |
| --- | --- | --- | --- |
| `limit` | Number | `1` | The highest number of groups that passes. `--limit` replaces it |

- The default mode is `warn`, so the rule never changes the exit code until you set `"mode": "fail"`.
- It gives the same result in a full and a partial check, and it runs no tests.

## Run goalie in CI

This GitHub Actions workflow runs every rule, and fails a pull request when a rule in `fail` mode reports a result:

```yaml
name: goalie

on:
  pull_request:

jobs:
  goalie:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: go install github.com/n-valchev/goalie/cmd/goalie@latest
      - run: "$(go env GOPATH)/bin/goalie check --base origin/${{ github.base_ref }}"
```

You must keep `fetch-depth: 0`. A rule with scope `changes` needs the merge base, and the default shallow clone doesn't contain it. Without the full history, goalie exits with `2` and prints this error for each rule:

```text
goalie: dup: no merge base between "origin/main" and HEAD: in a shallow clone, fetch more history
```

To adopt goalie without blocking work, set `"mode": "warn"` for each rule in `.goalierc` first. Change it to `"fail"` when you have checked the limits on your repository.

To split the check into one job for each rule, name the rule in each job, for example `goalie check dup --base origin/${{ github.base_ref }}`.

## Steer coding agents

A partial check runs no tests, so an agent can run it after each change. Add this instruction to the `CLAUDE.md` or `AGENTS.md` file of your repo:

```markdown
## Code checks

After you change Go code, run `goalie check --partial --base origin/main` from the repo root. Fix every result before you finish:

- `dup`: reuse the other function, or move the shared code to one place. Don't keep the copy.
- `crap ... can't pass at any coverage`: split the function. No test can bring it under the limit.
- `crap ... needs at least N% coverage`: write tests that cover at least that much of the function.
- `long-function`: split the function into smaller functions with clear names.
- `many-arguments` or `many-dependencies`: group parameters that belong together into a type, or split the function.
- `complex-condition`: give parts of the condition names, such as `ready := a && b`, or move the condition into a function.
- `cognitive-complexity`: return early in place of a nested `if`, or move a nested block into its own function.
- `primitive-arguments`: define a type for the value, such as `type UserID string`, and use it in each function that passes the value down.
- `long-file`: move a group of functions that belong together into a file of their own.
- `high-mean-complexity`: split the most complex functions, or move them into a file of their own.
- `low-cohesion`: move each group of methods, with the fields that it uses, into a type of its own.

Don't change `.goalierc` or `.goalieignore`, and don't add `//goalie:ignore`, unless a person asks you to.
```

- Use the branch that your pull requests target in place of `origin/main`.
- The instruction steers the agent, but it doesn't guard the code. Keep the full check in CI.

### Give the agent the MCP server

`goalie serve` runs an MCP server on stdin and stdout, so that an agent can call goalie as a tool. To add it to Claude Code for everyone who works on your repo, run this from the repo root:

```sh
claude mcp add --scope project goalie -- goalie serve
```

The command writes the server to `.mcp.json`. Commit that file. Each person approves the server the first time that Claude Code starts in the repo. To add the server for your own projects only, use `--scope user`.

The server has one tool:

| Tool | Input | Output |
| --- | --- | --- |
| `check` | `dir`, the absolute path of the folder that the agent works in. Optional: `scope` (`changes` or `all`, default `changes`), `path`, a package folder or a file relative to `dir`, and `base` | Up to 10 `results`, the worst first, with the fields of `goalie check --format json`. `more`, the count left out. `base`, the revision used. `unparsed`, the files that don't parse |

- The tool runs a partial check, and never runs your tests.
- It reads the `.goalierc` nearest to `dir`, so a worktree gets its own settings.
- It compares against the `base` input, then `base` in `.goalierc`, then `HEAD`. So it never fails because a revision is missing. With `scope` `all`, it uses no revision, and the answer holds no `base`.
- `path` replaces `paths` in `.goalierc`. An absolute `path` works too.
- It skips a file that doesn't parse, so that a half-finished edit doesn't fail the call. `goalie check` still exits with `2` on that file.
- It accepts a `dir` only in the repository where the server started, or in one of its worktrees.
- It reads the disk on each call, so an answer is never stale.

The server uses MCP version `2026-07-28`. It also answers a client that starts with `initialize`, with version `2025-11-25` or the older version that the client asks for.

## Map your packages

godeps reads your Go code and writes a map of it as one HTML file. You open the file in a browser, with no network and no install. It helps you see which code depends on a package, which imports break a rule, and where the risky and copied code is.

To make a map of the module in the current folder:

```sh
godeps > map.html
```

godeps checks nothing. arch-go stays your rule check in CI, and `goalie check` stays your other check. godeps only shows their reports on the map. Each report is optional.

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
| Red, dashed box, marked `!` | The worst function is above its limit, or the crap report left it out |
| Grey box | You gave no crap report |
| `2 duplicated functions` | Two functions in the box have a duplicate. With a dup report, `0` prints too. Boxes at the edge of an opened box show this count only in their tooltip |

A function that the crap report leaves out is red, so that unmeasured code does not look safe. A function with `//goalie:ignore crap <reason>` has no grade, because its author excused it, and its card shows `Excused from crap by //goalie:ignore`. A directive that names only other rules, such as `//goalie:ignore dup`, doesn't excuse it from a grade. An assembly stub has no grade, because the crap rule can't score it.

### Add the reports

Run these commands from the repo root. godeps expects the paths in the reports to start there:

```sh
arch-go --json || true
goalie check crap --all --limit 0 --shared-limit 0 --warn-only --format json > crap.json
goalie check dup --all --warn-only --format json > dup.json
godeps --arch-go-report .arch-go/report.json --crap-report crap.json --dup-report dup.json > map.html
```

| Command part | Why you need it |
| --- | --- |
| `\|\| true` after `arch-go` | arch-go exits with `1` when it finds a break, and it still writes the report. A script that stops on errors would otherwise never reach godeps. Keep your own arch-go step in CI as the gate |
| `--all` | Without it, a rule with scope `changes` reports only on code changed since `base`. The crap rule then leaves out every other function, and godeps shows them as red. The dup rule leaves out duplicates in old code, and they show no mark |
| `--limit 0` | Without it, the crap rule reports only functions above the limit, and godeps shows all the others as red |
| `--shared-limit 0` | Without it, shared functions keep their `sharedLimit` and drop out of the report. Leave this flag out if you set no `shared` patterns, because goalie then rejects it |
| `--warn-only` | Without it, goalie exits with `1` when a rule in `fail` mode reports anything |

Don't use `--partial` for a map. A partial check scores no function, so godeps exits with `2` on its report.

godeps takes the limits from `limit` and `sharedLimit` in `rules.crap` in `.goalierc`, or `30` if you set no `limit`. It ignores the limit in the report, which is `0` in this run. godeps checks these two values as goalie does. It exits with `2` on a negative `limit` or `sharedLimit`, and on a `sharedLimit` with no `shared` patterns.

To read `.arch-go/report.json`, godeps takes the imported package from the quoted text after `imported package` in each line. This depends on the message text of arch-go v2.

### Set the flags and settings

| Flag | Setting in the `godeps` object | Default | Meaning |
| --- | --- | --- | --- |
| `--format F` | `format` | `html` | Output format: `html` or `json` |
| `--arch-go-report <file>` | `archGoReport` | None | Mark the arrows that this arch-go report flags |
| `--crap-report <file>` | `crapReport` | None | Grade functions and boxes from this crap report |
| `--dup-report <file>` | `dupReport` | None | Mark functions from this dup report |
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
  "rules": { "crap": { "limit": 30, "sharedLimit": 15 } },
  "godeps": {
    "title": "Shop",
    "archGoReport": ".arch-go/report.json",
    "crapReport": "crap.json",
    "dupReport": "dup.json",
    "editorURL": "idea://open?file={file}&line={line}",
    "foreign": ["github.com/stripe/stripe-go", "go.uber.org/zap"],
    "omit": ["internal/gen/**"],
    "order": ["web", "billing", "shared"]
  }
}
```

- A flag replaces the setting of the same name. An empty report path means that the report is not set.
- `foreign`: an import at or below an entry draws an arrow to that entry. If entries overlap, the nearest one wins. An entry that nothing imports doesn't show. An entry never takes imports of the module that godeps scans.
- `omit`: each pattern is matched against the folder of a package, with the rules of [`skipFiles`](#skip-files-by-name). `gen/**` covers the folder `gen` and every folder below it. `gen` alone covers any folder named `gen`, and nothing below it. `.` covers the package in the root folder. godeps doesn't read the files of an omitted package, so they can't cause a parse error.
- `order`: names match the top-level boxes. The box of the shared code is named `shared`. A package at the repo root goes by its package clause, and a foreign module by its path.
- `shared`: a package is shared when any of its files matches. To match a folder, write `pkg/**`.

godeps reads `go.mod` from the directory it runs in, and from no other folder. It supports one module. It scans the same files as `goalie check crap`, so it skips [the same files](#choose-the-files-to-check) and leaves out test files. It leaves out imports of the standard library, imports of outside modules that are not in `foreign`, and imports of code that it didn't scan.

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
- A crap report with no `findings` array, or a finding with no file, line, or `score`. A report from `goalie check crap --partial` has no `score`.
- A dup report with no `findings` array, or a finding with no `left` or `right` file.
- An arch-go report with no `compliance` object.

These warnings go to stderr and don't change the exit code:

| Warning | What to do |
| --- | --- |
| `crap rows with no function: N` | Check that the report is from this code. Rows for omitted packages also count |
| `dup report sides with no function: N` | Check that the report is from this code. Sides in omitted packages or test files also count |
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
        },
        {
          "name": "generated",
          "file": "web/cart/cart.go",
          "line": 36,
          "endLine": 38,
          "exported": false,
          "url": "file:///repo/web/cart/cart.go",
          "ignored": true
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
| `packages[].duplicateCount` | Number | Functions at any depth with a duplicate. `0` with a dup report, if there are none. Left out with no report |
| `packages[].grade` | String | `green`, `yellow`, or `red`: the worst function at any depth. Left out with no crap report, or with no graded function |
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
| `classes[].ops[].name` | String | `Name` for a function, `Type.Name` for a method, as in `goalie check crap` |
| `classes[].ops[].file` | String | File from the run directory |
| `classes[].ops[].line` | Number | Line of the `func` keyword |
| `classes[].ops[].endLine` | Number | Last line of the function |
| `classes[].ops[].exported` | Boolean | Whether the function or method name is exported |
| `classes[].ops[].url` | String | Editor link |
| `classes[].ops[].crap` | Number | crap score. Left out if the report has no row for this function |
| `classes[].ops[].complexity` | Number | Cyclomatic complexity. Left out with `crap` |
| `classes[].ops[].coverage` | Number | A fraction from `0` to `1`. Left out with `crap` |
| `classes[].ops[].grade` | String | `green`, `yellow`, or `red`. `red` if the crap report has no row for it. Left out with no report, for an assembly stub, and for a function with `//goalie:ignore crap` that the report has no row for |
| `classes[].ops[].ignored` | Boolean | `true` for a function with `//goalie:ignore crap`. Left out otherwise |
| `classes[].ops[].duplicates[].id` | String | Class of the other function. Left out when it is not on the map, as with an omitted package or a test file |
| `classes[].ops[].duplicates[].name` | String | Name of the other function |
| `classes[].ops[].duplicates[].file` | String | File of the other function |
| `classes[].ops[].duplicates[].line` | Number | Line of the other function |
| `classes[].ops[].duplicates[].url` | String | Editor link |
| `classes[].ops[].duplicates[].score` | Number | dup score |
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
6. Double-click `web`, and then `cart`. The card lists the types, and the functions with their scores. `Sum` shows `CRAP 6.6`, `Skipped` shows `Excused from crap by //goalie:ignore`, and `Stub` shows no score. The duplicate mark of `Sum` names `Add` with its score, and `Gen` as not on the map.
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
goalie check dup --all --warn-only --format json > dup.json
```

## Credits

The `crap` rule follows [crap4go](https://github.com/unclebob/crap4go), and shares no code with it. Scores can differ from crap4go's scores, because goalie does not count `default` clauses as branches.

godeps follows the ideas and field names of [uml-viewer](https://github.com/unclebob/uml-viewer), and shares no code with it.

The `dup` rule follows the method of [dryer](https://github.com/unclebob/dryer) and [dry4clj](https://github.com/unclebob/dry4clj). Scores differ from dryer's scores, because goalie parses code with `go/parser` and the two parsers build different trees.

## License

goalie uses the [0BSD license](LICENSE).

# godry

godry finds Go functions with the same structure, so that you can catch a copied function before it merges. It compares functions across the whole repository, not one package at a time. It ships as one binary and uses only the Go standard library.

A renamed copy still matches. godry ignores the names of local variables, the names of fields, and the values of literals. It keeps the names of called functions and the operators, so functions that do different work don't match.

## Install godry

Choose one method.

| Method | Command | What you need |
| --- | --- | --- |
| Go | `go install github.com/n-valchev/godry@latest` | Go 1.25 or later |
| Nix, run once | `nix run github:n-valchev/godry -- --help` | Nix with flakes |
| Nix, keep on your `PATH` | `nix profile add github:n-valchev/godry` | Nix with flakes |

To check the install, run `godry --help`. The command prints the modes, flags, skip rules, and exit codes.

`--base` runs `git`. If you install with Go, you must have `git` on your `PATH`. The Nix package includes `git` and uses it only when your `PATH` has no `git`.

To add godry to your own Nix package set, use the overlay:

```nix
{
  inputs.godry.url = "github:n-valchev/godry";

  # In your NixOS or nixpkgs configuration:
  # nixpkgs.overlays = [ godry.overlays.default ];
  # After that, the package is pkgs.godry.
}
```

## Find duplicates

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

With no paths, godry uses the `paths` setting in `.godryrc`. If there is none, it scans the current directory tree. Paths limit both modes: godry never compares a function outside them.

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

Use `--format json` to get an object with a `findings` array. Each finding has `score`, `literals` (a number, or `null` for `-`), `anchors`, `left`, and `right`.

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
| `--help` | | Print usage |

godry has no baseline file, and its only settings files are `.godryrc` and `.godryignore`. It writes no files.

### Keep settings in a file

To keep your directories and settings in the repo, write them in a `.godryrc` file. The file holds one JSON object:

```json
{
  "paths": ["products/billing", "products/search", "pkg"],
  "threshold": 0.82,
  "weakAnchors": 1,
  "weakLiterals": 0.5,
  "skipFiles": ["packages.go", "internal/generated/**"],
  "skipTags": ["integration"]
}
```

Each setting is optional, and each one matches a flag. The same settings apply to all directories.

| Setting | Flag | Type |
| --- | --- | --- |
| `paths` | Paths on the command line | List of strings |
| `base` | `--base` | String |
| `threshold` | `--threshold` | Number |
| `minLines` | `--min-lines` | Whole number |
| `minNodes` | `--min-nodes` | Whole number |
| `weakAnchors` | `--weak-anchors` | Whole number |
| `weakLiterals` | `--weak-literals` | Number |
| `format` | `--format` | `"text"` or `"json"` |
| `warnOnly` | `--warn-only` | `true` or `false` |
| `includeTests` | `--include-tests` | `true` or `false` |
| `skipTags` | `--skip-tags` | List of strings |
| `skipFiles` | `--skip-files` | List of strings |

- godry reads `.godryrc` from the directory it runs in, and from no other folder. Paths and patterns start from that directory. Run godry from the repo root in CI.
- A flag replaces the same setting in the file. Paths on the command line replace `paths`. To turn off `warnOnly` for one run, use `--warn-only=false`.
- Patterns in `skipFiles`, `--skip-files`, and `.godryignore` all apply.
- A missing file is not an error. Invalid JSON, an unknown setting, or a bad value exits with `2`.
- JSON has no comments. To record why a file is skipped, put the pattern in `.godryignore`.
- If you set `base` in the file, every run uses new-code mode, and no flag turns it off. To keep whole-repo scans available, pass `--base` on the command line instead.

If agents work in your repo, require human approval for changes to `.godryrc` and `.godryignore`, for example with `CODEOWNERS`. Otherwise, an agent can lower the limits to pass the check.

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
- Files that match a pattern in `--skip-files` or in `.godryignore`.
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

To keep the patterns in the repo, write them in a `.godryignore` file:

```
# Dependency wiring repeats on purpose.
packages.go
wire_*.go
internal/legacy/client.go
```

- godry reads `.godryignore` from the directory it runs in, and from no other folder. Run it from the repo root in CI.
- Write one pattern per line. The patterns follow the table above. Blank lines and lines that start with `#` are ignored.
- `.godryignore` and `--skip-files` both apply.
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
      - run: go install github.com/n-valchev/godry@latest
      - run: "$(go env GOPATH)/bin/godry --base origin/${{ github.base_ref }}"
```

You must keep `fetch-depth: 0`. New-code mode needs the merge base, and the default shallow clone doesn't contain it. Without the full history, godry exits with `2` and prints this error:

```text
godry: no merge base between "origin/main" and HEAD: in a shallow clone, fetch more history
```

To adopt godry without blocking work, add `--warn-only` first. Remove it when you have checked the limits on your repository.

## Develop godry

1. Run `nix develop` to enter a shell with Go and `git`. You can also use your own Go 1.25 or later.
2. Run `go test ./...` to run the tests.
3. Run `nix build` to build the package. The build runs the same tests.

Nix sees only the files that Git tracks. Run `git add` on a new file before you run `nix build`.

## Credits

godry follows the method of [dryer](https://github.com/unclebob/dryer) and [dry4clj](https://github.com/unclebob/dry4clj). Scores differ from dryer's scores, because godry parses code with `go/parser` and the two parsers build different trees.

## License

godry uses the [0BSD license](LICENSE).

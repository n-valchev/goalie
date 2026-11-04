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

With no paths, godry scans the current directory tree. Paths limit both modes: godry never compares a function outside them.

### Read the output

godry prints one line for each pair:

```text
internal/a.go:12: duplicates internal/b.go:40 (score 0.91)
```

The score runs from `0` to `1`, and `1.00` means the two functions have the same structure. In new-code mode, the new function comes first. A clean scan prints `No duplicates found.`

Use `--format json` to get an object with a `findings` array.

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
| `--format F` | `text` | Output format: `text` or `json` |
| `--warn-only` | off | Exit with `0` when duplicates exist |
| `--include-tests` | off | Scan `*_test.go` files |
| `--skip-tags a,b` | none | Skip files that can only build with these tags |
| `--skip-files a,b` | none | Skip files that match these patterns |
| `--help` | | Print usage |

godry has no baseline file, and no settings file other than `.godryignore`. It writes no files.

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

A skipped file is not read, so it can't cause a parse error. Its functions are not compared on either side of a pair. You can repeat the flag or use a comma-separated list. A bad pattern exits with `2`.

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
- This is not full `.gitignore` syntax. `!`, `**`, and a trailing `/` exit with `2`, in the file and in the flag.

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

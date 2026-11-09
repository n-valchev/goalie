# Changelog

## Unreleased: `goalie check` replaces `godry` and `gocrap`

This release removes `godry` and `gocrap`. One command, `goalie check`, now runs both checks as the rules `dup` and `crap`. **CI jobs that run `godry` or `gocrap` break on this release.** Change them with the tables below.

To upgrade:

1. Install `goalie` in place of `godry` and `gocrap`. See [Install](#install).
2. Change each command in your CI jobs and scripts. See [Commands](#commands) and [Flags](#flags).
3. Move the `godry` and `gocrap` sections of `.goalierc` into `rules`. See [Settings](#settings).
4. Change `//godry:ignore` and `//gocrap:ignore` to `//goalie:ignore`. See [Directives](#directives).
5. If a `coverCommand` uses `GOCRAP_COVER_DIR`, change it to `GOALIE_COVER_DIR`.
6. Run `goalie check --base origin/main` and compare the results with your last CI run. They match, apart from two differences. Functions that a `//godry:ignore` or `//gocrap:ignore` line excused, and that you didn't change in step 4, now report. The new rules `long-function`, `many-arguments`, `many-dependencies`, `complex-condition`, `cognitive-complexity`, `primitive-arguments`, `long-file`, `high-mean-complexity`, and `low-cohesion` add `warn` results, which never change the exit code.

### Install

| Before | After |
| --- | --- |
| `go install github.com/n-valchev/goalie/cmd/godry@latest` | `go install github.com/n-valchev/goalie/cmd/goalie@latest` |
| `go install github.com/n-valchev/goalie/cmd/gocrap@latest` | `go install github.com/n-valchev/goalie/cmd/goalie@latest` |
| `nix run github:n-valchev/goalie#godry -- <args>` | `nix run github:n-valchev/goalie#goalie -- check dup <new args>` |
| `nix run github:n-valchev/goalie#gocrap -- <args>` | `nix run github:n-valchev/goalie#goalie -- check crap <new args>` |

Change the arguments with the [Commands](#commands) and [Flags](#flags) tables, because most old flags and all paths are now settings. The Nix package now has `goalie` as its main program. `godeps` is unchanged.

### Commands

A rule with scope `changes`, which is the default, needs a revision. The old commands checked the whole repository when you gave no `--base`, and no `base` was set in their section of `.goalierc`. To keep that, add `--all`. If the section set `base`, move it to the top level and leave out `--all`.

| Before | After |
| --- | --- |
| `godry` | `goalie check dup --all` |
| `godry --base <rev>` | `goalie check dup --base <rev>` |
| `gocrap` | `goalie check crap --all` |
| `gocrap --base <rev>` | `goalie check crap --base <rev>` |
| `godry --base <rev>` and `gocrap --base <rev>`, as two steps | `goalie check --base <rev>`, as one step |
| `godry --help`, `gocrap --help` | `goalie check --help` |

`goalie check` without rule names runs every rule that isn't `off`. If your `.goalierc` turns a rule off, name the rules that you want: `goalie check dup crap --base <rev>`.

### Flags

Flags that set a limit of a rule now work only when you name one rule. Every other flag became a setting in `.goalierc`.

| Before | After |
| --- | --- |
| `--base <rev>` | `--base <rev>` |
| `--format F` | `--format F` |
| `--help` | `--help` |
| `godry --threshold N` | `goalie check dup --limit N`, or `threshold` in `rules.dup` |
| `godry --min-lines N` | `minLines` in `rules.dup` |
| `godry --min-nodes N` | `minNodes` in `rules.dup` |
| `godry --weak-anchors N` | `weakAnchors` in `rules.dup` |
| `godry --weak-literals X` | `weakLiterals` in `rules.dup` |
| `godry --include-tests` | `includeTests` in `rules.dup` |
| `godry --warn-only` | `goalie check dup --warn-only`, or `"mode": "warn"` in `rules.dup` |
| `gocrap --limit N` | `goalie check crap --limit N`, or `limit` in `rules.crap` |
| `gocrap --shared-limit N` | `goalie check crap --shared-limit N`, or `sharedLimit` in `rules.crap` |
| `gocrap --cover-profiles a,b` | `coverProfiles` in `rules.crap`. Remove `coverCommand` from the file, because the two can't be set together |
| `gocrap --warn-only` | `goalie check crap --warn-only`, or `"mode": "warn"` in `rules.crap` |
| `--skip-tags a,b` | `skipTags` at the top level of `.goalierc` |
| `--skip-files a,b` | `skipFiles` at the top level of `.goalierc`, or a line for each pattern in `.goalieignore` |
| `--shared a,b` | `shared` at the top level of `.goalierc` |
| Paths, such as `godry pkg cmd` | `paths` at the top level of `.goalierc`. A word after `goalie check` is a rule name, so `goalie check pkg` exits with `2` |

`--warn-only` works only with one rule name, so `goalie check --warn-only` exits with `2`. To run every rule without failing, set `"mode": "warn"` for each rule in `.goalierc`.

### Settings

The `godry` and `gocrap` sections of `.goalierc` are now an error. goalie exits with `2` and names the new place:

```text
goalie: .goalierc: move "godry" settings to "rules.dup"
```

| Before | After |
| --- | --- |
| `godry.base`, `gocrap.base` | `base` at the top level. It applies to every rule with scope `changes`. `--all` turns it off for one run |
| `godry.threshold` | `rules.dup.threshold` |
| `godry.minLines` | `rules.dup.minLines` |
| `godry.minNodes` | `rules.dup.minNodes` |
| `godry.weakAnchors` | `rules.dup.weakAnchors` |
| `godry.weakLiterals` | `rules.dup.weakLiterals` |
| `godry.includeTests` | `rules.dup.includeTests` |
| `gocrap.limit` | `rules.crap.limit` |
| `gocrap.sharedLimit` | `rules.crap.sharedLimit` |
| `gocrap.coverCommand` | `rules.crap.coverCommand` |
| `gocrap.coverProfiles` | `rules.crap.coverProfiles` |
| `godry.warnOnly`, `gocrap.warnOnly` | `"mode": "warn"` in the rule. `warnOnly` in a rule is an error |
| `godry.format`, `gocrap.format` | `--format` on the command line. `format` in a rule is an error |

`paths`, `skipTags`, `skipFiles`, and `shared` stay at the top level. The defaults of every setting are unchanged.

This example shows one file before and after:

```json
{
  "shared": ["pkg/**"],
  "godry": { "base": "origin/main", "threshold": 0.85, "warnOnly": true },
  "gocrap": { "base": "origin/main", "limit": 30, "sharedLimit": 15 }
}
```

```json
{
  "base": "origin/main",
  "shared": ["pkg/**"],
  "rules": {
    "dup": { "threshold": 0.85, "mode": "warn" },
    "crap": { "limit": 30, "sharedLimit": 15 }
  }
}
```

Every rule also takes two new settings: `mode` (`off`, `warn`, or `fail`, default `fail` for `dup` and `crap`) and `scope` (`changes` or `all`, default `changes`). A mode change has the same effect as a limit change, so keep human approval for changes to `.goalierc`.

### Directives

| Before | After |
| --- | --- |
| `//godry:ignore <reason>` | `//goalie:ignore dup <reason>` |
| `//gocrap:ignore <reason>` | `//goalie:ignore crap <reason>` |
| Both, above one function | `//goalie:ignore crap,dup <reason>` |

`//godry:ignore` and `//gocrap:ignore` now have no effect, so the function is checked again. An unknown rule name or a missing reason in `//goalie:ignore` exits with `2`.

### Environment variables

| Before | After |
| --- | --- |
| `GOCRAP_COVER_DIR` | `GOALIE_COVER_DIR` |

goalie no longer sets `GOCRAP_COVER_DIR`. In a `coverCommand` that still uses it, `"$GOCRAP_COVER_DIR/coverage.out"` becomes `/coverage.out`, outside the folder that goalie reads. goalie then exits with `2` and skips `crap`. It prints `coverage command failed` if the command can't write there, or else `the coverage command wrote no profile to GOALIE_COVER_DIR`.

### Output

| Before | After |
| --- | --- |
| `a.go:12: duplicates b.go:40 (score 0.91, literals 0.20, anchors 3)` | `a.go:12: fail dup: duplicates b.go:40 (score 0.91, limit 0.82, literals 0.20, anchors 3)` |
| `a.go:12: Run scores 130.2 (complexity 12, coverage 45.0%, limit 30)` | `a.go:12: fail crap: Run scores 130.2 (complexity 12, coverage 45.0%, limit 30)` |
| `No duplicates found.` and `No functions over the limit.` on stdout | Nothing on stdout. A rule with nothing to check writes a note on stderr, such as `goalie: dup: No new functions found.` |
| Messages that start with `godry:` or `gocrap:` | Messages that start with `goalie:`. An error of one rule also names the rule, such as `goalie: dup: ...` |
| JSON `findings` of one tool | JSON `findings` of every rule in one list. Each entry also has `rule`, `mode`, `value`, and `limit`. The other fields are unchanged |
| Exit `1` when a check reports anything | Exit `1` only for a result in `fail` mode. A `warn` result never changes the exit code |

Results of all rules are in one list. `fail` results come first, then `warn` results, and within each the highest value divided by its limit comes first.

If your tests fail, `goalie check` still reports `dup`, skips `crap`, and exits with `2`. `gocrap` printed no scores in that case.

### `godeps`

| Before | After |
| --- | --- |
| `gocrap --limit 0 --shared-limit 0 --warn-only --format json > crap.json` | `goalie check crap --all --limit 0 --shared-limit 0 --warn-only --format json > crap.json` |
| `godry --format json --warn-only > dry.json` | `goalie check dup --all --warn-only --format json > dup.json` |
| `--dry-report <file>` | `--dup-report <file>` |
| `dryReport` in the `godeps` section | `dupReport` in the `godeps` section |
| Limits from `gocrap.limit` and `gocrap.sharedLimit` | Limits from `rules.crap.limit` and `rules.crap.sharedLimit` |
| A function with `//gocrap:ignore` has no grade | A function with `//goalie:ignore crap` has no grade |
| Card text `Ignored by gocrap` | Card text `Excused from crap by //goalie:ignore` |
| Warning `gocrap rows with no function: N` | Warning `crap rows with no function: N` |
| Warning `godry report sides with no function: N` | Warning `dup report sides with no function: N` |

godeps now exits with `2` on a crap report with no `score`. `goalie check crap --partial` writes such a report, and a map built from it would show every function as green.

### New in this release

- `goalie check --partial` runs no tests. `dup` gives its exact result, and `crap` reports from complexity alone: a function that can't pass at any coverage, or the coverage that a function needs.
- `goalie check --all` checks the whole repository and needs no revision.
- `goalie serve` runs an MCP server for coding agents. Its tool `check` runs a partial check and gives the 10 worst results, the count of the rest, the revision used, and the files that don't parse. Each result says whether the change made it `new`, made it `worse`, or left it `unchanged`, and `unchanged` results come last. Each result also has a `hint`, one line on how to fix it. It needs no revision: with no `base` input and no `base` in `.goalierc`, it uses `HEAD`. See the README section "Give the agent the MCP server".
- `mode` and `scope` for each rule.
- A rule that you name runs even if it is `off`, and reports as `warn`.
- Nine new rules report code that is hard to change:

  | Rule | Reports | Default limit |
  | --- | --- | --- |
  | `long-function` | A function with more lines than the limit, from the `func` keyword to the closing brace | 80 |
  | `many-arguments` | A function or method with more parameters than the limit. A constructor uses `many-dependencies` instead | 4 |
  | `many-dependencies` | A constructor with more parameters than the limit. A constructor is a function named `New` or `New` and a capital letter, such as `NewServer` | 5 |
  | `complex-condition` | A function with an `if`, `for`, or `case` condition that has more `&&` and `\|\|` operators than the limit. A function reports once, at its worst condition | 3 |
  | `cognitive-complexity` | A function that is hard to follow. Each branch adds 1, plus 1 for each block that it is inside. A Go guard, such as `if err != nil { return err }`, adds 0 | 15 |
  | `primitive-arguments` | A function with a parameter of a basic type, such as `string`, that it only passes down to other code in your repo. A later change of the type then changes every function that passes the value down | 1 |
  | `long-file` | A file with more lines than the limit. With scope `changes`, a file reports only when the change takes it over the limit, or when it is new. An old long file gives no result | 1000 |
  | `high-mean-complexity` | A file whose functions have a mean complexity over the limit. Complexity is the one that `crap` uses. Scope `changes` works as for `long-file` | 7 |
  | `low-cohesion` | A struct type whose methods fall into more groups than the limit. Two methods are in one group when they use the same field or one calls the other. An accessor is left out. An accessor only returns a receiver field, a field of that field, or the address of either, such as `return s.conn`. A call to an accessor counts as a use of its field. Excuse a type with `//goalie:ignore low-cohesion <reason>` above it | 1 |

  Set each limit with `limit` in the rule's object in `rules`. Their default mode is `warn`, so they never change the exit code until you set `"mode": "fail"`. Their default scope is `changes`, so they need a revision. If your `.goalierc` sets no `base` and gives `dup` and `crap` scope `all` or mode `off`, `goalie check` with no `--base` now exits with `2`. To fix this, give the nine new rules the same scope or mode.

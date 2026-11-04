package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// git runs in dir with the machine's own Git config hidden, so that a developer's settings can't
// change what a test sets up. Run itself still sees the real config.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Dev", "GIT_AUTHOR_EMAIL=dev@example.com",
		"GIT_COMMITTER_NAME=Dev", "GIT_COMMITTER_EMAIL=dev@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := writeTree(t, files)
	git(t, dir, "init", "--initial-branch=main")
	commitAll(t, dir, "first")
	return dir
}

func commitAll(t *testing.T, dir, message string) {
	t.Helper()
	git(t, dir, "add", "--all")
	git(t, dir, "commit", "--allow-empty", "--message", message)
}

func wantReport(t *testing.T, dir string, wantCode int, want string, args ...string) {
	t.Helper()
	code, stdout, stderr := runGodry(t, dir, args...)
	if code != wantCode {
		t.Errorf("exit code = %d, want %d; stderr: %s", code, wantCode, stderr)
	}
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// This is the case dryer missed: its changed-files check never compared new code with the rest
// of the repo, and found nothing once the code was committed.
func TestCommittedCopyOfAnOldFunctionIsReported(t *testing.T) {
	dir := gitRepo(t, map[string]string{"old.go": "package a\n\n" + sumFunc("Old")})
	git(t, dir, "checkout", "-b", "feature")
	write(t, dir, "zcopy.go", "package a\n\n"+sumFunc("Copy"))
	commitAll(t, dir, "copy")

	wantReport(t, dir, 1, "zcopy.go:3: duplicates old.go:3 (score 1.00, literals 1.00, anchors 0)\n", "--base", "main")
}

func TestUncommittedCopyIsReported(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"old.go":   "package a\n\n" + sumFunc("Old"),
		"zwork.go": "package a\n\n" + joinFunc("Work"),
	})
	write(t, dir, "zwork.go", "package a\n\n"+joinFunc("Work")+"\n"+sumFunc("Copy"))

	wantReport(t, dir, 1, "zwork.go:14: duplicates old.go:3 (score 1.00, literals 1.00, anchors 0)\n", "--base", "main")
}

func TestStagedCopyIsReported(t *testing.T) {
	dir := gitRepo(t, map[string]string{"old.go": "package a\n\n" + sumFunc("Old")})
	write(t, dir, "zcopy.go", "package a\n\n"+sumFunc("Copy"))
	git(t, dir, "add", "zcopy.go")

	wantReport(t, dir, 1, "zcopy.go:3: duplicates old.go:3 (score 1.00, literals 1.00, anchors 0)\n", "--base", "main")
}

func TestUntrackedCopyIsReported(t *testing.T) {
	dir := gitRepo(t, map[string]string{"old.go": "package a\n\n" + sumFunc("Old")})
	write(t, dir, "zcopy.go", "package a\n\n"+sumFunc("Copy"))

	wantReport(t, dir, 1, "zcopy.go:3: duplicates old.go:3 (score 1.00, literals 1.00, anchors 0)\n", "--base", "main")
}

func TestIgnoredFileIsNotNew(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		".gitignore": "ignored.go\n",
		"old.go":     "package a\n\n" + sumFunc("Old"),
	})
	write(t, dir, "ignored.go", "package a\n\n"+sumFunc("Copy"))

	wantReport(t, dir, 0, "No new functions found.\n", "--base", "main")
}

func TestOldPairsAreLeftOut(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"old1.go": "package a\n\n" + sumFunc("One"),
		"old2.go": "package a\n\n" + sumFunc("Two"),
	})
	write(t, dir, "new.go", "package a\n\n"+joinFunc("Fresh"))

	wantReport(t, dir, 0, "No duplicates found.\n", "--base", "main")
}

func TestAnEditedFunctionWithAnOldDuplicateIsReported(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"a_old.go": "package a\n\n" + sumFunc("Old"),
		"edit.go":  "package a\n\n" + sumFunc("Edited"),
		"z_old.go": "package a\n\n" + joinFunc("Other"),
	})
	edited := strings.Replace(sumFunc("Edited"), "return 100", "return 200", 1)
	write(t, dir, "edit.go", "package a\n\n"+edited)

	wantReport(t, dir, 1, "edit.go:3: duplicates a_old.go:3 (score 1.00, literals 0.50, anchors 0)\n", "--base", "main")
}

func TestABothNewPairIsReportedOnceInLocationOrder(t *testing.T) {
	dir := gitRepo(t, map[string]string{"old.go": "package a\n\n" + joinFunc("Old")})
	write(t, dir, "z.go", "package a\n\n"+sumFunc("Second"))
	write(t, dir, "b.go", "package a\n\n"+sumFunc("First"))

	wantReport(t, dir, 1, "b.go:3: duplicates z.go:3 (score 1.00, literals 1.00, anchors 0)\n", "--base", "main")
}

func TestAChangeOutsideAFunctionSpanMarksNothing(t *testing.T) {
	body := sumFunc("Work")
	dir := gitRepo(t, map[string]string{
		"old.go":  "package a\n\n" + sumFunc("Old"),
		"work.go": "package a\n\n// Work adds.\n" + body + "\nvar limit = 1\n",
	})
	write(t, dir, "work.go", "package a\n\n// Work adds values.\n"+body+"\nvar limit = 2\n")

	wantReport(t, dir, 0, "No new functions found.\n", "--base", "main")
}

func TestADeletionInsideAFunctionMarksIt(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"a_old.go": "package a\n\n" + sumFunc("Old"),
		"work.go":  "package a\n\n" + sumFunc("Work"),
	})
	shorter := strings.Replace(sumFunc("Work"), "\tif total > 100 {\n\t\treturn 100\n\t}\n", "", 1)
	write(t, dir, "work.go", "package a\n\n"+shorter)

	wantReport(t, dir, 1, "work.go:3: duplicates a_old.go:3 (score 0.79, literals 0.00, anchors 0)\n",
		"--base", "main", "--threshold", "0.5")
}

func TestDeletingAFunctionDoesNotMarkItsNeighbors(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"old.go":  "package a\n\n" + sumFunc("Old"),
		"work.go": "package a\n\n" + sumFunc("Before") + "var gone = 1\n" + sumFunc("After"),
	})
	write(t, dir, "work.go", "package a\n\n"+sumFunc("Before")+sumFunc("After"))

	wantReport(t, dir, 0, "No new functions found.\n", "--base", "main")
}

func TestDeletingAFileIsNotAnError(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"gone.go": "package a\n\n" + sumFunc("Gone"),
		"old.go":  "package a\n\n" + sumFunc("Old"),
	})
	if err := os.Remove(filepath.Join(dir, "gone.go")); err != nil {
		t.Fatal(err)
	}

	wantReport(t, dir, 0, "No new functions found.\n", "--base", "main")
}

func TestARenamedFileCountsAsNew(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"old.go": "package a\n\n" + sumFunc("Old"),
		"src.go": "package a\n\n" + sumFunc("Moved"),
	})
	git(t, dir, "checkout", "-b", "feature")
	git(t, dir, "mv", "src.go", "zdst.go")
	commitAll(t, dir, "rename")

	wantReport(t, dir, 1, "zdst.go:3: duplicates old.go:3 (score 1.00, literals 1.00, anchors 0)\n", "--base", "main")
}

func TestAFunctionMovedToAnotherFileCountsAsNew(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"old.go": "package a\n\n" + sumFunc("Old"),
		"src.go": "package a\n\n" + joinFunc("Stay") + "\n" + sumFunc("Moved"),
	})
	write(t, dir, "src.go", "package a\n\n"+joinFunc("Stay"))
	write(t, dir, "zdst.go", "package a\n\n"+sumFunc("Moved"))
	git(t, dir, "add", "--all")

	wantReport(t, dir, 1, "zdst.go:3: duplicates old.go:3 (score 1.00, literals 1.00, anchors 0)\n", "--base", "main")
}

func TestChangesOnTheBaseBranchAfterTheMergeBaseDoNotCount(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"old1.go": "package a\n\n" + sumFunc("One"),
		"old2.go": "package a\n\n" + sumFunc("Two"),
	})
	git(t, dir, "checkout", "-b", "feature")
	git(t, dir, "checkout", "main")
	edited := strings.Replace(sumFunc("One"), "return 100", "return 200", 1)
	write(t, dir, "old1.go", "package a\n\n"+edited)
	commitAll(t, dir, "later work on main")
	git(t, dir, "checkout", "feature")

	wantReport(t, dir, 0, "No new functions found.\n", "--base", "main")
}

func TestPathsLimitNewCodeMode(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"in/old.go":  "package a\n\n" + sumFunc("InOld"),
		"out/old.go": "package a\n\n" + joinFunc("OutOld"),
	})
	write(t, dir, "in/zcopy.go", "package a\n\n"+sumFunc("InCopy"))
	write(t, dir, "out/zcopy.go", "package a\n\n"+joinFunc("OutCopy"))

	wantReport(t, dir, 1, "in/zcopy.go:3: duplicates in/old.go:3 (score 1.00, literals 1.00, anchors 0)\n", "--base", "main", "in")
}

func TestAChangedFunctionOutsideThePathsIsNotCompared(t *testing.T) {
	dir := gitRepo(t, map[string]string{"in/old.go": "package a\n\n" + sumFunc("Old")})
	write(t, dir, "out/copy.go", "package a\n\n"+sumFunc("Copy"))

	wantReport(t, dir, 0, "No new functions found.\n", "--base", "main", "in")
}

func TestANewFunctionWhoseOnlyDuplicateIsOutsideThePathsIsClean(t *testing.T) {
	dir := gitRepo(t, map[string]string{"out/old.go": "package a\n\n" + sumFunc("Old")})
	write(t, dir, "in/copy.go", "package a\n\n"+sumFunc("Copy"))

	wantReport(t, dir, 0, "No duplicates found.\n", "--base", "main", "in")
}

func TestPathsStayRelativeToASubdirectoryOfTheRepo(t *testing.T) {
	tests := []struct {
		name   string
		commit bool
	}{
		{"untracked", false},
		{"committed", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := gitRepo(t, map[string]string{"sub/old.go": "package a\n\n" + sumFunc("Old")})
			write(t, dir, "sub/zcopy.go", "package a\n\n"+sumFunc("Copy"))
			write(t, dir, "zroot.go", "package a\n\n"+joinFunc("Root"))
			write(t, dir, "zzroot.go", "package a\n\n"+joinFunc("OtherRoot"))
			if test.commit {
				git(t, dir, "checkout", "-b", "feature")
				commitAll(t, dir, "copies")
			}
			git(t, dir, "config", "diff.relative", "true")
			sub := filepath.Join(dir, "sub")

			wantReport(t, sub, 1, "zcopy.go:3: duplicates old.go:3 (score 1.00, literals 1.00, anchors 0)\n", "--base", "main")
			wantReport(t, sub, 1, "../zroot.go:3: duplicates ../zzroot.go:3 (score 1.00, literals 1.00, anchors 3)\n"+
				"zcopy.go:3: duplicates old.go:3 (score 1.00, literals 1.00, anchors 0)\n", "--base", "main", "..")
		})
	}
}

func wantError(t *testing.T, dir, wantStderr string, args ...string) {
	t.Helper()
	code, stdout, stderr := runGodry(t, dir, args...)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, wantStderr) {
		t.Errorf("stderr = %q, want it to contain %q", stderr, wantStderr)
	}
}

func TestAnUnknownRevisionIsAnError(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n"})

	wantError(t, dir, `unknown revision "nope"`, "--base", "nope")
	wantError(t, dir, `unknown revision "nope"`, "--base", "nope", "--warn-only")
}

func TestAnEmptyBaseIsAnErrorNotAWholeRepoScan(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n"})

	wantError(t, dir, "must name a revision", "--base", "")
}

func TestARevisionThatLooksLikeAnOptionIsNotPassedToGit(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n"})

	wantError(t, dir, "unknown revision", "--base=--output=written.txt")
	wantError(t, dir, "unknown revision", "--base=--default=HEAD")

	if _, err := os.Stat(filepath.Join(dir, "written.txt")); !os.IsNotExist(err) {
		t.Errorf("git wrote a file: %v", err)
	}
}

func TestBaseOutsideAGitRepoIsAnError(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": "package a\n"})

	wantError(t, dir, "--base needs a Git repository", "--base", "main")
	wantError(t, dir, "--base needs a Git repository", "--base", "main", "--warn-only")
}

func TestGitErrorsComeBeforeTheNoFilesMessage(t *testing.T) {
	dir := gitRepo(t, map[string]string{"README.md": "# Not Go\n"})

	wantError(t, dir, "unknown revision", "--base", "nope")
}

func TestUnrelatedHistoriesHaveNoMergeBase(t *testing.T) {
	dir := gitRepo(t, map[string]string{"a.go": "package a\n"})
	git(t, dir, "checkout", "--orphan", "island")
	commitAll(t, dir, "unrelated")

	wantError(t, dir, "no merge base", "--base", "main")
}

func TestAShallowCloneWithoutTheMergeBaseSaysToFetch(t *testing.T) {
	source := gitRepo(t, map[string]string{"a.go": "package a\n"})
	git(t, source, "checkout", "-b", "feature")
	commitAll(t, source, "on feature")
	git(t, source, "checkout", "main")
	commitAll(t, source, "on main")
	clone := filepath.Join(t.TempDir(), "clone")
	git(t, t.TempDir(), "clone", "--depth=1", "--no-single-branch", "file://"+source, clone)
	git(t, clone, "checkout", "feature")

	wantError(t, clone, "fetch more history", "--base", "origin/main")
	wantError(t, clone, "fetch more history", "--base", "origin/main", "--warn-only")
}

func TestPatchLikeSourceLinesAreNotReadAsHeaders(t *testing.T) {
	edited := strings.Replace(sumFunc("Work"), "return 100", "return 200", 1)
	dir := gitRepo(t, map[string]string{
		"a_old.go": "package a\n\n" + sumFunc("Old"),
		"work.go":  "package a\n\nconst text = ``\n\n" + sumFunc("Work"),
	})
	write(t, dir, "work.go", "package a\n\nconst text = `\n++ b/elsewhere.go\n`\n\n"+edited)

	wantReport(t, dir, 1, "work.go:7: duplicates a_old.go:3 (score 1.00, literals 0.50, anchors 0)\n", "--base", "main")
}

func TestFileNamesThatGitQuotesOrPadsAreFound(t *testing.T) {
	names := []string{
		"with space.go",
		"naïve.go",
		"日本語/ファイル.go",
		`quo"te.go`,
		`back\slash.go`,
		"tab\tname.go",
		"new\nline.go",
		"both \"kinds\" ü.go",
	}
	edited := strings.Replace(sumFunc("Copy"), "return 100", "return 200", 1)
	for _, name := range names {
		want := name + ":3: duplicates a_old.go:3 (score 1.00, literals 1.00, anchors 0)\n"
		wantEdited := name + ":3: duplicates a_old.go:3 (score 1.00, literals 0.50, anchors 0)\n"
		old := map[string]string{"a_old.go": "package a\n\n" + sumFunc("Old")}

		t.Run("untracked "+name, func(t *testing.T) {
			dir := gitRepo(t, old)
			write(t, dir, name, "package a\n\n"+sumFunc("Copy"))

			wantReport(t, dir, 1, want, "--base", "main")
		})
		t.Run("edited "+name, func(t *testing.T) {
			dir := gitRepo(t, map[string]string{
				"a_old.go": old["a_old.go"],
				name:       "package a\n\n" + sumFunc("Copy"),
			})
			write(t, dir, name, "package a\n\n"+edited)

			wantReport(t, dir, 1, wantEdited, "--base", "main")
		})
	}
}

// settingsRepo has one old pair, and an uncommitted edit in the first and the last function of
// work.go. The function between them must stay old. Any context line, merged hunk, or lost file
// name shows up as a different report.
func settingsRepo(t *testing.T) string {
	t.Helper()
	work := func(first, last string) string {
		return "package a\n\n" + first + "\n" + sumFunc("Keep") + "\n" + last
	}
	dir := gitRepo(t, map[string]string{
		"other.go": "package a\n\n" + sumFunc("Other") + "\n" + joinFunc("OtherJoin"),
		"work.go":  work(joinFunc("First"), joinFunc("Last")),
	})
	edit := func(name string) string {
		return strings.Replace(joinFunc(name), `", "`, `"; "`, 1)
	}
	write(t, dir, "work.go", work(edit("First"), edit("Last")))
	return dir
}

const settingsReport = "work.go:3: duplicates other.go:16 (score 1.00, literals 0.00, anchors 3)\n" +
	"work.go:3: duplicates work.go:27 (score 1.00, literals 1.00, anchors 3)\n" +
	"work.go:27: duplicates other.go:16 (score 1.00, literals 0.00, anchors 3)\n"

func TestDiffSettingsHaveNoEffect(t *testing.T) {
	tests := []struct {
		name       string
		config     []string
		attributes string
	}{
		{name: "defaults"},
		{name: "no prefix", config: []string{"diff.noprefix", "true"}},
		{name: "mnemonic prefix", config: []string{"diff.mnemonicPrefix", "true"}},
		{name: "custom prefixes", config: []string{"diff.srcPrefix", "x/", "diff.dstPrefix", "y/"}},
		{name: "context lines", config: []string{"diff.context", "10"}},
		{name: "inter-hunk context", config: []string{"diff.interHunkContext", "40"}},
		{name: "external diff", config: []string{"diff.external", "false"}},
		{name: "unquoted paths", config: []string{"core.quotePath", "false"}},
		{name: "color", config: []string{"color.ui", "always", "color.diff", "always"}},
		{name: "algorithm", config: []string{"diff.algorithm", "patience"}},
		{name: "no indent heuristic", config: []string{"diff.indentHeuristic", "false"}},
		{name: "copies", config: []string{"diff.renames", "copies"}},
		{name: "binary attribute", attributes: "*.go -diff\n"},
		{
			name:       "driver command",
			config:     []string{"diff.fake.command", "false"},
			attributes: "*.go diff=fake\n",
		},
		{
			name:       "driver text conversion",
			config:     []string{"diff.fake.textconv", "sed 1,8d"},
			attributes: "*.go diff=fake\n",
		},
		{
			name:       "driver marks binary",
			config:     []string{"diff.fake.binary", "true"},
			attributes: "*.go diff=fake\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := settingsRepo(t)
			for i := 0; i < len(test.config); i += 2 {
				git(t, dir, "config", test.config[i], test.config[i+1])
			}
			write(t, dir, ".git/info/attributes", test.attributes)

			wantReport(t, dir, 1, settingsReport, "--base", "main")
		})
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestATouchedFileIsNotNewAndTheIndexStaysUnchanged(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"old.go":  "package a\n\n" + sumFunc("Old"),
		"work.go": "package a\n\n" + sumFunc("Work"),
	})
	touch(t, filepath.Join(dir, "work.go"))
	index := filepath.Join(dir, ".git", "index")
	before := readFile(t, index)

	wantReport(t, dir, 0, "No new functions found.\n", "--base", "main")

	if readFile(t, index) != before {
		t.Error("the run rewrote .git/index")
	}
}

func TestEveryGitCommandLeavesTheIndexUnchanged(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"old.go":         "package a\n\n" + sumFunc("Old"),
		"work.go":        "package a\n\n" + joinFunc("Work"),
		composed + ".go": "package a\n\n" + joinFunc("Tracked"),
	})
	touch(t, filepath.Join(dir, "work.go"))
	write(t, dir, "zcopy.go", "package a\n\n"+sumFunc("Copy"))
	write(t, dir, decomposed+"2.go", "package a\n\n"+sumFunc("Untracked"))
	index := filepath.Join(dir, ".git", "index")
	before := readFile(t, index)

	code, _, stderr := runGodry(t, dir, "--base", "main")

	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr)
	}
	if readFile(t, index) != before {
		t.Error("the run rewrote .git/index")
	}
}

func TestATouchedFileInASubmoduleLeavesItsIndexUnchanged(t *testing.T) {
	inner := gitRepo(t, map[string]string{"inner.go": "package inner\n"})
	dir := gitRepo(t, map[string]string{"old.go": "package a\n\n" + sumFunc("Old")})
	git(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", inner, "sub")
	commitAll(t, dir, "add submodule")
	touch(t, filepath.Join(dir, "sub", "inner.go"))
	index := filepath.Join(dir, ".git", "modules", "sub", "index")
	before := readFile(t, index)

	wantReport(t, dir, 0, "No new functions found.\n", "--base", "HEAD")

	if readFile(t, index) != before {
		t.Error("the run rewrote the index of the submodule")
	}
}

func TestNoNewFunctionsInJSONKeepsStdoutParseable(t *testing.T) {
	dir := gitRepo(t, map[string]string{"old.go": "package a\n\n" + sumFunc("Old")})

	code, stdout, stderr := runGodry(t, dir, "--base", "main", "--format", "json")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	assertEmptyFindings(t, stdout)
	if stderr != "No new functions found.\n" {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestJSONMarksOnlyTheNewFunctionAsNew(t *testing.T) {
	dir := gitRepo(t, map[string]string{"old.go": "package a\n\n" + sumFunc("Old")})
	write(t, dir, "zcopy.go", "package a\n\n"+sumFunc("Copy"))

	_, stdout, _ := runGodry(t, dir, "--base", "main", "--format", "json")

	var report struct {
		Findings []struct {
			Left, Right struct {
				File string
				New  bool
			}
		}
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout = %q: %v", stdout, err)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("stdout = %q, want one finding", stdout)
	}
	finding := report.Findings[0]
	if finding.Left.File != "zcopy.go" || !finding.Left.New {
		t.Errorf("left = %+v, want new zcopy.go", finding.Left)
	}
	if finding.Right.File != "old.go" || finding.Right.New {
		t.Errorf("right = %+v, want old old.go", finding.Right)
	}
}

func TestASkippedFileCannotContributeNewFunctions(t *testing.T) {
	dir := gitRepo(t, map[string]string{"old.go": "package a\n\n" + sumFunc("Old")})
	write(t, dir, "copy_test.go", "package a\n\n"+sumFunc("Copy"))

	wantReport(t, dir, 0, "No new functions found.\n", "--base", "main")
	wantReport(t, dir, 1, "copy_test.go:3: duplicates old.go:3 (score 1.00, literals 1.00, anchors 0)\n",
		"--base", "main", "--include-tests")
}

func TestTheSubmoduleDiffSettingHasNoEffect(t *testing.T) {
	inner := gitRepo(t, map[string]string{"inner.go": "package inner\n"})
	dir := gitRepo(t, map[string]string{"old.go": "package a\n\n" + sumFunc("Old")})
	git(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", inner, "sub")
	commitAll(t, dir, "add submodule")
	write(t, dir, "sub/copy.go", "package inner\n\n"+sumFunc("Copy"))
	commitAll(t, filepath.Join(dir, "sub"), "copy inside the submodule")
	git(t, dir, "config", "diff.submodule", "diff")

	wantReport(t, dir, 0, "No new functions found.\n", "--base", "HEAD")
}

func TestGitEnvironmentHasNoEffect(t *testing.T) {
	elsewhere := gitRepo(t, map[string]string{"z.go": "package z\n"})
	tests := []struct{ name, value string }{
		{"GIT_EXTERNAL_DIFF", "false"},
		{"GIT_DIFF_OPTS", "--unified=10"},
		{"GIT_DIR", filepath.Join(elsewhere, ".git")},
		{"GIT_WORK_TREE", elsewhere},
		{"GIT_INDEX_FILE", filepath.Join(elsewhere, ".git", "index")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := settingsRepo(t)
			t.Setenv(test.name, test.value)

			wantReport(t, dir, 1, settingsReport, "--base", "main")
		})
	}
}

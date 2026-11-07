package goalie_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// One name in its two Unicode forms. macOS keeps whichever form a program wrote, and Git may store
// or print the other one.
const (
	composed   = "café"
	decomposed = "café"
)

var precomposeValues = []string{"true", "false"}

// requireFormBlindFilesystem skips a test that needs one file to answer to both forms. On other
// filesystems the two forms are two separate files.
func requireFormBlindFilesystem(t *testing.T) {
	t.Helper()
	dir := writeTree(t, map[string]string{decomposed: ""})
	if _, err := os.Stat(filepath.Join(dir, composed)); err != nil {
		t.Skip("this filesystem treats the two Unicode forms as different names")
	}
}

func precomposeRepo(t *testing.T, value string, files map[string]string) string {
	t.Helper()
	dir := writeTree(t, files)
	git(t, dir, "init", "--initial-branch=main")
	git(t, dir, "config", "core.precomposeunicode", value)
	commitAll(t, dir, "first")
	return dir
}

func TestUntrackedDecomposedNameIsNew(t *testing.T) {
	for _, value := range precomposeValues {
		t.Run(value, func(t *testing.T) {
			dir := precomposeRepo(t, value, map[string]string{"a_old.go": "package a\n\n" + sumFunc("Old")})
			write(t, dir, decomposed+".go", "package a\n\n"+sumFunc("Copy"))

			wantReport(t, dir, 1, decomposed+".go:3: fail dup: duplicates a_old.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n", "--base", "main")
		})
	}
}

func TestCommittedDecomposedNameIsNew(t *testing.T) {
	for _, value := range precomposeValues {
		t.Run(value, func(t *testing.T) {
			dir := precomposeRepo(t, value, map[string]string{"a_old.go": "package a\n\n" + sumFunc("Old")})
			git(t, dir, "checkout", "-b", "feature")
			write(t, dir, decomposed+".go", "package a\n\n"+sumFunc("Copy"))
			commitAll(t, dir, "copy")

			wantReport(t, dir, 1, decomposed+".go:3: fail dup: duplicates a_old.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n", "--base", "main")
		})
	}
}

func TestDecomposedFolderIsNew(t *testing.T) {
	for _, value := range precomposeValues {
		t.Run(value, func(t *testing.T) {
			dir := precomposeRepo(t, value, map[string]string{"a_old.go": "package a\n\n" + sumFunc("Old")})
			write(t, dir, decomposed+"/copy.go", "package a\n\n"+sumFunc("Copy"))

			wantReport(t, dir, 1, decomposed+"/copy.go:3: fail dup: duplicates a_old.go:3 (score 1.00, limit 0.82, literals 1.00, anchors 0)\n", "--base", "main")
		})
	}
}

// formsRepo stores a file's name in one form, then leaves it on disk in another form, so that Git
// and the disk disagree. The setting takes its final value only after the commit, because with it
// on, git add rewrites the stored name.
func formsRepo(t *testing.T, stored, onDisk, precompose, content string) string {
	t.Helper()
	dir := precomposeRepo(t, "false", map[string]string{
		"a_old.go":     "package a\n\n" + sumFunc("Old"),
		stored + ".go": content,
	})
	if err := os.Remove(filepath.Join(dir, stored+".go")); err != nil {
		t.Fatal(err)
	}
	write(t, dir, onDisk+".go", content)
	git(t, dir, "config", "core.precomposeunicode", precompose)
	return dir
}

func forEachForm(t *testing.T, test func(t *testing.T, stored, onDisk, precompose string)) {
	t.Helper()
	requireFormBlindFilesystem(t)
	forms := []struct{ name, value string }{{"NFC", composed}, {"NFD", decomposed}}
	for _, stored := range forms {
		for _, onDisk := range forms {
			for _, precompose := range precomposeValues {
				t.Run("stored "+stored.name+" on disk "+onDisk.name+" precompose "+precompose, func(t *testing.T) {
					test(t, stored.value, onDisk.value, precompose)
				})
			}
		}
	}
}

func TestAnUnchangedFileIsOldWhateverFormItsNameHas(t *testing.T) {
	forEachForm(t, func(t *testing.T, stored, onDisk, precompose string) {
		dir := formsRepo(t, stored, onDisk, precompose, "package a\n\n"+sumFunc("Keep"))

		wantReport(t, dir, 0, "No new functions found.\n", "--base", "main")
	})
}

func TestAnEditIsFoundWhateverFormTheNameHas(t *testing.T) {
	forEachForm(t, func(t *testing.T, stored, onDisk, precompose string) {
		keep := joinFunc("Keep") + "\n"
		dir := formsRepo(t, stored, onDisk, precompose, "package a\n\n"+keep+sumFunc("Edit"))
		edited := strings.Replace(sumFunc("Edit"), "return 100", "return 200", 1)
		write(t, dir, onDisk+".go", "package a\n\n"+keep+edited)

		wantReport(t, dir, 1, onDisk+".go:14: fail dup: duplicates a_old.go:3 (score 1.00, limit 0.82, literals 0.50, anchors 0)\n", "--base", "main")
	})
}

func TestAScanFromAFolderWhoseNameHasTheOtherForm(t *testing.T) {
	requireFormBlindFilesystem(t)
	for _, precompose := range precomposeValues {
		t.Run(precompose, func(t *testing.T) {
			dir := precomposeRepo(t, "true", map[string]string{
				decomposed + "/a_old.go": "package a\n\n" + sumFunc("Old"),
				decomposed + "/work.go":  "package a\n\n" + sumFunc("Work"),
			})
			git(t, dir, "config", "core.precomposeunicode", precompose)
			edited := strings.Replace(sumFunc("Work"), "return 100", "return 200", 1)
			write(t, dir, decomposed+"/work.go", "package a\n\n"+edited)

			wantReport(t, filepath.Join(dir, decomposed), 1, "work.go:3: fail dup: duplicates a_old.go:3 (score 1.00, limit 0.82, literals 0.50, anchors 0)\n", "--base", "main")
		})
	}
}

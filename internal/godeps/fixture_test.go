package godeps_test

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

const fixtureDir = "testdata/page"

var update = flag.Bool("update", false, "rewrite testdata/page/map.html")

// Editor links hold the absolute path of the checkout, which differs on each machine.
var checkoutPath = regexp.MustCompile(`file:///[^"]*?/testdata/page/`)

func fixturePath(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// The rows for gen are on purpose: gen is omitted, so that a card shows a copy that is not on the
// map. Any other count means that the reports no longer match the sources.
func TestTheFixturePageIsCurrent(t *testing.T) {
	stdout, stderr := run(t, fixturePath(t), 0)
	const warnings = "godeps: godry report sides with no function: 2\ngodeps: gocrap rows with no function: 1\n"
	if stderr != warnings {
		t.Errorf("stderr = %q, want %q", stderr, warnings)
	}

	page := checkoutPath.ReplaceAllString(stdout, "file:///goalie/testdata/page/")
	file := filepath.Join(fixtureDir, "map.html")
	if *update {
		if err := os.WriteFile(file, []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if page != string(want) {
		t.Errorf("%s is out of date. To rebuild it, run: go test ./internal/godeps -run FixturePage -update", file)
	}
}

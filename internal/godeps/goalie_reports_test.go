package godeps_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/n-valchev/goalie/internal/goalie"
)

const crapFixture = "testdata/crap"

// The README test reads these too, so that the commands it gives are the ones tested here.
var (
	crapCommand = []string{"check", "crap", "--all", "--limit", "0", "--shared-limit", "0", "--warn-only", "--format", "json"}
	dupCommand  = []string{"check", "dup", "--all", "--warn-only", "--format", "json"}
)

func runGoalie(t *testing.T, dir string, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := goalie.Run(args, dir, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("goalie exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	return stdout.String()
}

// The grade and duplicate tests read these fixtures, so they show what godeps makes of real output.
func TestTheFixtureReportsAreWhatGoalieCheckPrints(t *testing.T) {
	cases := []struct {
		dir, report string
		args        []string
	}{
		{crapFixture, "crap.json", crapCommand},
		{dupFixture, "dup.json", dupCommand},
		// The page has no crap case, because its module holds an import loop that go test can't build.
		{fixtureDir, "dup.json", dupCommand},
	}
	for _, tc := range cases {
		t.Run(tc.dir, func(t *testing.T) {
			stdout := runGoalie(t, tc.dir, tc.args...)
			file := filepath.Join(tc.dir, tc.report)
			if *update {
				if err := os.WriteFile(file, []byte(stdout), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if stdout != string(want) {
				t.Errorf("%s is out of date. To rebuild it, run: go test ./internal/godeps -run FixtureReports -update", file)
			}
		})
	}
}

func TestARealPartialCrapReportIsAnError(t *testing.T) {
	stdout := runGoalie(t, crapFixture, append(slices.Clone(crapCommand), "--partial")...)
	report := filepath.Join(t.TempDir(), "crap.json")
	if err := os.WriteFile(report, []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	wantError(t, crapFixture, "has no score", "--format", "json", "--crap-report", report)
}

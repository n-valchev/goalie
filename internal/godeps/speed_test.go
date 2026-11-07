package godeps_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The target in the spec.
const mapBudget = 5 * time.Second

const (
	syntheticProducts = 20
	syntheticFolders  = 20
	syntheticFiles    = 10
	syntheticTypes    = 3
	syntheticFuncs    = 12
)

type syntheticCrapRow struct {
	File       string  `json:"file"`
	Line       int     `json:"line"`
	Function   string  `json:"function"`
	Score      float64 `json:"score"`
	Complexity int     `json:"complexity"`
	Coverage   float64 `json:"coverage"`
	Limit      float64 `json:"limit"`
}

type syntheticSide struct {
	File      string `json:"file"`
	StartLine int    `json:"startLine"`
	Function  string `json:"function"`
}

type syntheticPair struct {
	Score float64       `json:"score"`
	Left  syntheticSide `json:"left"`
	Right syntheticSide `json:"right"`
}

func syntheticFolder(index int) string {
	index %= syntheticProducts * syntheticFolders
	return fmt.Sprintf("product%02d/area%02d", index/syntheticFolders, index%syntheticFolders)
}

// syntheticFile returns a file of about 170 lines that imports folders near its own, so that most
// products have a few neighbours, as in a real repo. It also returns the line of each function.
func syntheticFile(folder, number int) (string, []int) {
	var text strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&text, format+"\n", args...) }
	line("package area%02d\n", folder%syntheticFolders)
	line("import (")
	for step := 1; step <= 5; step++ {
		line("\t_ \"example.com/shop/%s\"", syntheticFolder(folder+step*step*(number+1)))
	}
	line(")\n")
	for kind := range syntheticTypes {
		line("type Record%d%d struct {\n\tID string\n\tTotal int\n\tNext *Record%d%d\n}\n", number, kind, number, kind)
	}
	var lines []int
	for function := range syntheticFuncs {
		lines = append(lines, strings.Count(text.String(), "\n")+1)
		line("func Work%d%d(values []int, label string) int {", number, function)
		line("\ttotal := 0\n\tfor _, value := range values {\n\t\tif value > %d {\n\t\t\ttotal += value\n\t\t}\n\t}", function)
		line("\tif label == \"\" {\n\t\treturn total\n\t}\n\treturn total + len(label)\n}\n")
	}
	return text.String(), lines
}

func toJSON(t *testing.T, findings any) string {
	t.Helper()
	text, err := json.Marshal(map[string]any{"findings": findings})
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

// syntheticModule writes one gocrap row for every function and one godry pair for every file, so
// that the run pays for both joins.
func syntheticModule(t *testing.T) (dir string, fileCount int) {
	t.Helper()
	files := map[string]string{"go.mod": goMod}
	rows := []syntheticCrapRow{}
	pairs := []syntheticPair{}
	for folder := range syntheticProducts * syntheticFolders {
		for number := range syntheticFiles {
			name := fmt.Sprintf("%s/file%d.go", syntheticFolder(folder), number)
			text, lines := syntheticFile(folder, number)
			files[name] = text
			for function, line := range lines {
				rows = append(rows, syntheticCrapRow{name, line, fmt.Sprintf("Work%d%d", number, function), 6, 3, 0.5, 0})
			}
			left := syntheticSide{name, lines[0], fmt.Sprintf("Work%d0", number)}
			right := syntheticSide{name, lines[1], fmt.Sprintf("Work%d1", number)}
			pairs = append(pairs, syntheticPair{0.9, left, right})
		}
	}
	fileCount = len(files) - 1
	files["crap.json"] = toJSON(t, rows)
	files["dup.json"] = toJSON(t, pairs)
	return writeTree(t, files), fileCount
}

func TestAModuleOfFourThousandFilesMapsWithinBudget(t *testing.T) {
	dir, fileCount := syntheticModule(t)

	start := time.Now()
	stdout, stderr := run(t, dir, 0, "--crap-report", "crap.json", "--dup-report", "dup.json")
	elapsed := time.Since(start)

	t.Logf("%d files, %d MB page, %v", fileCount, len(stdout)>>20, elapsed)
	if stderr != "" {
		t.Errorf("stderr = %q, want no warnings", stderr)
	}
	if fileCount != 4000 {
		t.Errorf("%d files, want 4000", fileCount)
	}
	if elapsed > mapBudget {
		t.Errorf("the run took %v, want at most %v", elapsed, mapBudget)
	}
}

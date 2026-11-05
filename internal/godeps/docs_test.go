package godeps_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	exampleBlock = regexp.MustCompile("(?s)```json\n(.*?)```")
	fieldRow     = regexp.MustCompile("(?m)^\\| `([^`]+)` \\|")
	flagName     = regexp.MustCompile(`--[a-z][a-z-]+`)
)

var everySetting = []string{
	"paths", "skipTags", "skipFiles", "shared",
	"format", "archGoReport", "crapReport", "dryReport", "editorURL", "title", "foreign", "omit", "order",
}

func between(t *testing.T, text, heading string) string {
	t.Helper()
	_, after, found := strings.Cut(text, heading+"\n")
	if !found {
		t.Fatalf("no heading %q", heading)
	}
	level := heading[:strings.Index(heading, " ")+1]
	section, _, _ := strings.Cut(after, "\n"+level)
	return section
}

func readmeSection(t *testing.T) string {
	t.Helper()
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	return between(t, string(readme), "## Map your packages")
}

// jsonFields maps each path with a value, such as classes[].ops[].crap, to the kind of that value.
func jsonFields(value any, path string, fields map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			jsonFields(child, strings.TrimPrefix(path+"."+key, "."), fields)
		}
	case []any:
		for _, child := range typed {
			jsonFields(child, path+"[]", fields)
		}
	default:
		fields[path] = fmt.Sprintf("%T", value)
	}
}

func parseFields(t *testing.T, text string) map[string]string {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
	fields := map[string]string{}
	jsonFields(value, "", fields)
	return fields
}

// The fixture module sets every field, so a field that the README lacks or that no longer exists
// shows up here in one direction or the other.
func TestTheREADMEModelMatchesWhatJSONPrints(t *testing.T) {
	stdout, _ := run(t, fixturePath(t), 0, "--format", "json")
	want := parseFields(t, stdout)
	section := between(t, readmeSection(t), "### Read the model")

	example := exampleBlock.FindStringSubmatch(section)
	if example == nil {
		t.Fatal("the model section has no JSON example")
	}
	if got := parseFields(t, example[1]); !maps.Equal(got, want) {
		t.Errorf("the example has the fields\n%v\nwant\n%v", got, want)
	}

	var table []string
	for _, row := range fieldRow.FindAllStringSubmatch(section, -1) {
		table = append(table, row[1])
	}
	slices.Sort(table)
	if names := slices.Sorted(maps.Keys(want)); !slices.Equal(table, names) {
		t.Errorf("the table lists\n%v\nwant\n%v", table, names)
	}
}

func TestTheREADMENamesEveryFlagAndSetting(t *testing.T) {
	section := readmeSection(t)
	help, _ := run(t, t.TempDir(), 0, "--help")
	for _, want := range append(flagName.FindAllString(help, -1), everySetting...) {
		if !strings.Contains(section, want) {
			t.Errorf("the README section does not name %q", want)
		}
	}
}

func TestTheREADMEStatesTheCommandsThatReachGodeps(t *testing.T) {
	section := readmeSection(t)
	for _, want := range []string{
		"arch-go --json || true",
		"gocrap --limit 0 --shared-limit 0 --warn-only --format json > crap.json",
		"godry --format json --warn-only > dry.json",
		"idea://", "zed://", "subl://", "txmt://", "mvim://", "Neovim",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("the README section does not state %q", want)
		}
	}
	if strings.Contains(section, "gocrap --limit 0 --warn-only") {
		t.Error("the README section has the gocrap command that drops shared functions")
	}
}

func TestHelpNamesEverySettingAndWhatExitsWithTwo(t *testing.T) {
	stdout, _ := run(t, t.TempDir(), 0, "--help")
	for _, want := range append(slices.Clone(everySetting), "limit", "sharedLimit", "base", "go.mod", "doesn't parse", "isn't valid") {
		if !strings.Contains(stdout, want) {
			t.Errorf("--help does not name %q", want)
		}
	}
}

package godeps_test

import (
	"encoding/json"
	"testing"
)

const linkedSource = "package shop\n\ntype Order struct{}\n\nfunc (o Order) Total() int { return 0 }\n"

type links struct {
	Classes []struct {
		Types []struct {
			URL string `json:"url"`
		} `json:"types"`
		Ops []struct {
			URL string `json:"url"`
		} `json:"ops"`
	} `json:"classes"`
}

// wantLinks checks the single type on line 3 and the single op on line 5 of linkedSource.
func wantLinks(t *testing.T, dir, wantType, wantOp string, args ...string) {
	t.Helper()
	stdout, _ := run(t, dir, 0, append([]string{"--format", "json"}, args...)...)
	var result links
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not a model: %v\n%s", err, stdout)
	}
	if len(result.Classes) != 1 || len(result.Classes[0].Types) != 1 || len(result.Classes[0].Ops) != 1 {
		t.Fatalf("want one class with one type and one op, got %s", stdout)
	}
	class := result.Classes[0]
	if got := class.Types[0].URL; got != wantType {
		t.Errorf("type url = %q, want %q", got, wantType)
	}
	if got := class.Ops[0].URL; got != wantOp {
		t.Errorf("op url = %q, want %q", got, wantOp)
	}
}

func TestLinksDefaultToTheFileWithNoLine(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "billing/order.go": linkedSource})
	want := "file://" + dir + "/billing/order.go"
	wantLinks(t, dir, want, want)
}

func awkwardTree(t *testing.T) string {
	t.Helper()
	return writeTree(t, map[string]string{"go.mod": goMod, "a b&c#d/my order.go": linkedSource})
}

func TestDefaultLinkEncodesAPathWithASpaceAmpersandAndHash(t *testing.T) {
	dir := awkwardTree(t)
	want := "file://" + dir + "/a%20b%26c%23d/my%20order.go"
	wantLinks(t, dir, want, want)
}

func TestQueryTemplateKeepsItsOwnSeparatorsAndEncodesTheValues(t *testing.T) {
	dir := awkwardTree(t)
	file := dir + "/a%20b%26c%23d/my%20order.go"
	wantLinks(t, dir, "idea://open?file="+file+"&line=3", "idea://open?file="+file+"&line=5",
		"--editor-url", "idea://open?file={file}&line={line}")
}

func TestPlusAndPercentInAPathSurviveDecoding(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "c++/100%.go": linkedSource})
	want := "file://" + dir + "/c%2B%2B/100%25.go"
	wantLinks(t, dir, want, want)
}

func TestAPlaceholderInAPathIsNotReplaced(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "{line}/order.go": linkedSource})
	wantLinks(t, dir, "x:"+dir+"/%7Bline%7D/order.go:3", "x:"+dir+"/%7Bline%7D/order.go:5",
		"--editor-url", "x:{file}:{line}")
}

func TestTemplateWithNoLineIsValid(t *testing.T) {
	dir := writeTree(t, map[string]string{"go.mod": goMod, "order.go": linkedSource})
	want := "subl://open?url=" + dir + "/order.go"
	wantLinks(t, dir, want, want, "--editor-url", "subl://open?url={file}")
}

func TestEditorURLSettingSetsTheTemplate(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"order.go":  linkedSource,
		".goalierc": `{"godeps": {"editorURL": "zed://file{file}:{line}"}}`,
	})
	wantLinks(t, dir, "zed://file"+dir+"/order.go:3", "zed://file"+dir+"/order.go:5")
}

func TestEditorURLFlagReplacesTheSetting(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"order.go":  linkedSource,
		".goalierc": `{"godeps": {"editorURL": "zed://file{file}:{line}"}}`,
	})
	wantLinks(t, dir, "x:"+dir+"/order.go:3", "x:"+dir+"/order.go:5", "--editor-url", "x:{file}:{line}")
}

func TestEmptyEditorURLSettingUsesTheDefault(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"order.go":  linkedSource,
		".goalierc": `{"godeps": {"editorURL": ""}}`,
	})
	want := "file://" + dir + "/order.go"
	wantLinks(t, dir, want, want)
}

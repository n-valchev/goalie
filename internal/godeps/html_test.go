package godeps_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var modelElement = regexp.MustCompile(`(?s)<script type="application/json" id="model">(.*?)</script>`)

func pageModel(t *testing.T, page string) any {
	t.Helper()
	found := modelElement.FindStringSubmatch(page)
	if found == nil {
		t.Fatalf("no model element in the page:\n%s", page)
	}
	var result any
	if err := json.Unmarshal([]byte(found[1]), &result); err != nil {
		t.Fatalf("the model element is not JSON: %v\n%s", err, found[1])
	}
	return result
}

func wantPageHoldsTheModel(t *testing.T, dir string, args ...string) {
	t.Helper()
	stdout, _ := run(t, dir, 0, append([]string{"--format", "json"}, args...)...)
	var want any
	if err := json.Unmarshal([]byte(stdout), &want); err != nil {
		t.Fatal(err)
	}
	page, _ := run(t, dir, 0, append([]string{"--format", "html"}, args...)...)
	if got := pageModel(t, page); !reflect.DeepEqual(got, want) {
		t.Errorf("page model = %v, want %v", got, want)
	}
}

func mapTree(t *testing.T) string {
	t.Helper()
	return writeTree(t, map[string]string{
		"go.mod":           goMod,
		".goalierc":        `{"godeps": {"foreign": ["example.com/lib"], "order": ["web"]}, "shared": ["lib/**"]}`,
		"main.go":          "package main\n\nimport _ \"example.com/shop/web\"\n",
		"web/web.go":       "package web\n\nimport _ \"example.com/shop/lib/text\"\n\ntype Cart struct{ ID string }\n\nfunc Open() {}\n",
		"lib/text/text.go": "package text\n\nimport _ \"example.com/lib/sub\"\n",
	})
}

func TestThePageHoldsTheModelThatJSONPrints(t *testing.T) {
	wantPageHoldsTheModel(t, mapTree(t))
}

func TestThePageLoadsNothingFromTheNetwork(t *testing.T) {
	page, _ := run(t, mapTree(t), 0)
	remote := regexp.MustCompile(`(?i)\b(src|href)\s*=\s*["']?\s*(https?:)?//`)
	if found := remote.FindString(page); found != "" {
		t.Errorf("the page has the remote reference %q", found)
	}
}

func TestTextInTheModelCanNotEndTheModelElement(t *testing.T) {
	const hostile = "</script><b>"
	dir := writeTree(t, map[string]string{"go.mod": goMod, hostile + "/a.go": "package a\n\nfunc A() {}\n"})
	wantPageHoldsTheModel(t, dir, "--title", hostile)

	page, _ := run(t, dir, 0, "--title", hostile)
	if got := strings.Count(page, "</script>"); got != 2 {
		t.Errorf("the page has %d script end tags, want 2: one for the model and one for the code", got)
	}
	if strings.Contains(page, "<b>") {
		t.Error("the title reached the page as markup")
	}
}

func TestTheTitleNamesThePage(t *testing.T) {
	page, _ := run(t, smallTree(t, "{}"), 0, "--title", "Shop & Co")
	if !strings.Contains(page, "<title>Shop &amp; Co</title>") {
		t.Errorf("the page has no title element for the title:\n%s", page)
	}
}

func TestTheSameInputGivesTheSamePage(t *testing.T) {
	dir := mapTree(t)
	first, _ := run(t, dir, 0)
	second, _ := run(t, dir, 0)
	if first != second {
		t.Error("two runs wrote different pages")
	}
}

func TestHelpOffersHTML(t *testing.T) {
	stdout, _ := run(t, t.TempDir(), 0, "--help")
	if strings.Contains(stdout, "not available") {
		t.Errorf("help still says that html is not available:\n%s", stdout)
	}
}

// This only parses the code. The behaviour of the page is checked by hand, as the spec decides.
func TestThePageCodeParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page, _ := run(t, mapTree(t), 0)
	found := regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindStringSubmatch(page)
	if found == nil {
		t.Fatal("no code element in the page")
	}
	path := filepath.Join(t.TempDir(), "page.js")
	if err := os.WriteFile(path, []byte(found[1]), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(node, "--check", path).CombinedOutput(); err != nil {
		t.Errorf("node --check failed: %v\n%s", err, output)
	}
}

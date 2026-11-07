package godeps_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const dupFixture = "testdata/dup"

type duplicate struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	File  string  `json:"file"`
	Line  int     `json:"line"`
	URL   string  `json:"url"`
	Score float64 `json:"score"`
}

type dryOp struct {
	Name       string      `json:"name"`
	Duplicates []duplicate `json:"duplicates"`
}

type dryClass struct {
	ID             string  `json:"id"`
	DuplicateCount *int    `json:"duplicateCount"`
	Ops            []dryOp `json:"ops"`
}

type dryBox struct {
	ID             string `json:"id"`
	DuplicateCount *int   `json:"duplicateCount"`
}

type dryModel struct {
	Packages []dryBox   `json:"packages"`
	Classes  []dryClass `json:"classes"`
}

func (m dryModel) op(t *testing.T, class, name string) dryOp {
	t.Helper()
	for _, found := range m.Classes {
		for _, op := range found.Ops {
			if found.ID == class && op.Name == name {
				return op
			}
		}
	}
	t.Fatalf("no op %s in class %s: %+v", name, class, m.Classes)
	return dryOp{}
}

func TestAnOpInTwoPairsGetsADuplicateForEachOtherSide(t *testing.T) {
	result := readModel[dryModel](t, dupFixture, "--dup-report", "dup.json", "--editor-url", "idea://open?file={file}&line={line}")
	got := result.op(t, "example.com/shop/billing/invoice", "Total").Duplicates
	want := []duplicate{
		{"example.com/shop/orders", "Order.Sum", "orders/order.go", 5, "idea://open?file=testdata/dup/orders/order.go&line=5", 1},
		{"example.com/shop/web/cart", "Subtotal", "web/cart/cart.go", 3, "idea://open?file=testdata/dup/web/cart/cart.go&line=3", 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("duplicates = %+v, want %+v", got, want)
	}
}

func TestADuplicateLinkUsesTheDefaultEditorURL(t *testing.T) {
	result := readModel[dryModel](t, dupFixture, "--dup-report", "dup.json")
	got := result.op(t, "example.com/shop/orders", "Order.Sum").Duplicates[0].URL
	if want := "file://testdata/dup/billing/invoice/total.go"; got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
}

func wantCount(t *testing.T, what string, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Errorf("%s has no duplicateCount, want %d", what, want)
	} else if *got != want {
		t.Errorf("%s duplicateCount = %d, want %d", what, *got, want)
	}
}

const repeated = `func %NAME%(values []int) int {
	return len(values)
}
`

func pairReport(left, right string) string {
	return `{"findings": [{"score": 0.9,
		"left": {"file": "` + left + `", "function": "F", "startLine": 3},
		"right": {"file": "` + right + `", "function": "G", "startLine": 3}}]}`
}

func dryTree(t *testing.T, extra map[string]string) string {
	t.Helper()
	tree := map[string]string{
		"go.mod":     goMod,
		"root.go":    "package shop\n\n" + strings.ReplaceAll(repeated, "%NAME%", "F"),
		"a/b/one.go": "package b\n\n" + strings.ReplaceAll(repeated, "%NAME%", "F") + "\n" + strings.ReplaceAll(repeated, "%NAME%", "Clean"),
		"a/two.go":   "package a\n\n" + strings.ReplaceAll(repeated, "%NAME%", "G"),
		"c/c.go":     "package c\n\n" + strings.ReplaceAll(repeated, "%NAME%", "Clean"),
	}
	for name, content := range extra {
		tree[name] = content
	}
	return writeTree(t, tree)
}

func TestAnOpInTwoPairsCountsOnce(t *testing.T) {
	result := readModel[dryModel](t, dupFixture, "--dup-report", "dup.json")
	for _, class := range result.Classes {
		wantCount(t, class.ID, class.DuplicateCount, 1)
	}
	want := map[string]int{"billing": 1, "billing/invoice": 1, "orders": 1, "web": 1, "web/cart": 1}
	for _, box := range result.Packages {
		wantCount(t, box.ID, box.DuplicateCount, want[box.ID])
	}
}

func TestDuplicateCountAddsUpAtAnyDepthAndShowsZero(t *testing.T) {
	dir := dryTree(t, map[string]string{"dup.json": pairReport("a/b/one.go", "a/two.go")})
	result := readModel[dryModel](t, dir, "--dup-report", "dup.json")
	want := map[string]int{"a": 2, "a/b": 1, "c": 0}
	for _, box := range result.Packages {
		wantCount(t, box.ID, box.DuplicateCount, want[box.ID])
	}
	wantClass := map[string]int{"example.com/shop": 0, "example.com/shop/a": 1, "example.com/shop/a/b": 1, "example.com/shop/c": 0}
	for _, class := range result.Classes {
		wantCount(t, class.ID, class.DuplicateCount, wantClass[class.ID])
	}
}

func TestDuplicateCountReachesTheSharedBox(t *testing.T) {
	dir := dryTree(t, map[string]string{"dup.json": pairReport("a/b/one.go", "root.go")})
	result := readModel[dryModel](t, dir, "--dup-report", "dup.json", "--shared", "a/**")
	want := map[string]int{":shared": 1, ":shared/a": 1, ":shared/a/b": 1, "c": 0}
	for _, box := range result.Packages {
		wantCount(t, box.ID, box.DuplicateCount, want[box.ID])
	}
}

func TestAForeignClassHasNoDuplicateCount(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod":    goMod,
		"a/a.go":    "package a\n\nimport _ \"example.com/lib\"\n",
		".goalierc": `{"godeps": {"foreign": ["example.com/lib"]}}`,
		"dup.json":  `{"findings": []}`,
	})
	for _, class := range readModel[dryModel](t, dir, "--dup-report", "dup.json").Classes {
		if class.ID == "example.com/lib" && class.DuplicateCount != nil {
			t.Errorf("foreign class duplicateCount = %d, want none", *class.DuplicateCount)
		}
	}
}

func TestWithNoDryReportTheModelHasNoDuplicateData(t *testing.T) {
	stdout, _ := run(t, dupFixture, 0, "--format", "json")
	for _, field := range []string{"duplicates", "duplicateCount"} {
		if strings.Contains(stdout, field) {
			t.Errorf("stdout holds %q with no report:\n%s", field, stdout)
		}
	}
}

func TestAMissingDryReportFails(t *testing.T) {
	_, stderr := run(t, dryTree(t, nil), 2, "--format", "json", "--dup-report", "nowhere.json")
	if !strings.Contains(stderr, "nowhere.json") {
		t.Errorf("stderr = %q, want the path named", stderr)
	}
}

func TestAnInvalidDryReportFails(t *testing.T) {
	tests := []struct{ name, content string }{
		{"not json", "oops"},
		{"empty", ""},
		{"no findings", "{}"},
		{"null findings", `{"findings": null}`},
		{"a crap report", `{"limit": 8, "findings": [{"file": "root.go", "line": 3, "crap": 2}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := dryTree(t, map[string]string{"dup.json": test.content})
			_, stderr := run(t, dir, 2, "--format", "json", "--dup-report", "dup.json")
			if !strings.Contains(stderr, "dup.json") {
				t.Errorf("stderr = %q, want the path named", stderr)
			}
		})
	}
}

func TestTheDryReportComesFromTheSettingUnlessTheFlagReplacesIt(t *testing.T) {
	dir := dryTree(t, map[string]string{
		".goalierc": `{"godeps": {"dupReport": "dup.json"}}`,
		"dup.json":  pairReport("a/b/one.go", "a/two.go"),
		"none.json": `{"findings": []}`,
	})
	if got := readModel[dryModel](t, dir).op(t, "example.com/shop/a", "G").Duplicates; len(got) != 1 {
		t.Errorf("from the setting: duplicates = %+v, want one", got)
	}
	if got := readModel[dryModel](t, dir, "--dup-report", "none.json").op(t, "example.com/shop/a", "G").Duplicates; len(got) != 0 {
		t.Errorf("from the flag: duplicates = %+v, want none", got)
	}
}

func TestAnAbsoluteDryReportPathIsUsedAsItIs(t *testing.T) {
	report := filepath.Join(writeTree(t, map[string]string{"dup.json": pairReport("a/b/one.go", "a/two.go")}), "dup.json")
	if got := readModel[dryModel](t, dryTree(t, nil), "--dup-report", report).op(t, "example.com/shop/a", "G").Duplicates; len(got) != 1 {
		t.Errorf("duplicates = %+v, want one", got)
	}
}

func TestSidesThatMatchNoOpWarnOnceWithTheCount(t *testing.T) {
	report := `{"findings": [
		{"score": 0.9, "left": {"file": "gone.go", "function": "F", "startLine": 3}, "right": {"file": "gone.go", "function": "G", "startLine": 9}},
		{"score": 0.9, "left": {"file": "a/two.go", "function": "G", "startLine": 99}, "right": {"file": "a/two_test.go", "function": "H", "startLine": 3}}
	]}`
	dir := dryTree(t, map[string]string{"dup.json": report})
	_, stderr := run(t, dir, 0, "--format", "json", "--dup-report", "dup.json")
	if want := "godeps: dup report sides with no function: 4\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestAReportThatMatchesPrintsNoWarning(t *testing.T) {
	if _, stderr := run(t, dupFixture, 0, "--format", "json", "--dup-report", "dup.json"); stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
}

func TestAnOpKeepsItsMarkWhenTheOtherSideIsOffTheMap(t *testing.T) {
	dir := dryTree(t, map[string]string{
		"dup.json":   pairReport("a/two.go", "gen/gen.go"),
		"gen/gen.go": "package gen\n\n" + strings.ReplaceAll(repeated, "%NAME%", "G"),
		".goalierc":  `{"godeps": {"omit": ["gen"]}}`,
	})
	stdout, stderr := run(t, dir, 0, "--format", "json", "--dup-report", "dup.json", "--editor-url", "x:{line}")
	if want := "godeps: dup report sides with no function: 1\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if strings.Contains(stdout, `"id": ""`) {
		t.Errorf("stdout holds an empty id:\n%s", stdout)
	}
	var result dryModel
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	got := result.op(t, "example.com/shop/a", "G").Duplicates
	want := []duplicate{{"", "G", "gen/gen.go", 3, "x:3", 0.9}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("duplicates = %+v, want %+v", got, want)
	}
}

func TestADuplicateOnALineWithTwoFunctionsGoesToTheNamedOne(t *testing.T) {
	tests := []struct{ left, right, leftOp, rightOp, leftOther, rightOther string }{
		{"F", "H", "F", "T.H", "G", "T.M"},
		{"G", "M", "G", "T.M", "F", "T.H"},
	}
	for _, test := range tests {
		t.Run(test.left+" and "+test.right, func(t *testing.T) {
			dir := writeTree(t, map[string]string{
				"go.mod": goMod,
				"a/a.go": "package a\n\nfunc F() {}; func G() {}\n",
				"b/b.go": "package b\n\ntype T struct{}\n\nfunc (T) H() {}; func (T) M() {}\n",
				"dup.json": `{"findings": [{"score": 0.9,
					"left": {"file": "a/a.go", "function": "` + test.left + `", "startLine": 3},
					"right": {"file": "b/b.go", "function": "` + test.right + `", "startLine": 5}}]}`,
			})
			got := readModel[dryModel](t, dir, "--dup-report", "dup.json")
			left := got.op(t, "example.com/shop/a", test.leftOp).Duplicates
			if len(left) != 1 || left[0].Name != test.rightOp || left[0].ID != "example.com/shop/b" {
				t.Errorf("%s duplicates = %+v, want %s in b", test.leftOp, left, test.rightOp)
			}
			right := got.op(t, "example.com/shop/b", test.rightOp).Duplicates
			if len(right) != 1 || right[0].Name != test.leftOp || right[0].ID != "example.com/shop/a" {
				t.Errorf("%s duplicates = %+v, want %s in a", test.rightOp, right, test.leftOp)
			}
			if other := got.op(t, "example.com/shop/a", test.leftOther).Duplicates; len(other) != 0 {
				t.Errorf("%s duplicates = %+v, want none", test.leftOther, other)
			}
			if other := got.op(t, "example.com/shop/b", test.rightOther).Duplicates; len(other) != 0 {
				t.Errorf("%s duplicates = %+v, want none", test.rightOther, other)
			}
		})
	}
}

func TestOneFunctionKeepsEveryPairOnItsLine(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"go.mod": goMod,
		"a/a.go": "package a\n\nfunc F() {}; func G() {}\n",
		"b/b.go": "package b\n\nfunc H() {}\n\nfunc M() {}\n",
		"dup.json": `{"findings": [
			{"score": 0.9, "left": {"file": "a/a.go", "function": "F", "startLine": 3}, "right": {"file": "b/b.go", "function": "H", "startLine": 3}},
			{"score": 0.8, "left": {"file": "a/a.go", "function": "F", "startLine": 3}, "right": {"file": "b/b.go", "function": "M", "startLine": 5}}]}`,
	})
	got := readModel[dryModel](t, dir, "--dup-report", "dup.json")
	if duplicates := got.op(t, "example.com/shop/a", "F").Duplicates; len(duplicates) != 2 {
		t.Errorf("F duplicates = %+v, want two", duplicates)
	}
}

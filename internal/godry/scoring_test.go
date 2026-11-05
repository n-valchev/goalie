package godry_test

import (
	"fmt"
	"strings"
	"testing"
)

func compareFuncs(t *testing.T, left, right string, args ...string) (code int, stdout string) {
	t.Helper()
	dir := writeTree(t, map[string]string{
		"left.go":  "package left\n\n" + left,
		"right.go": "package right\n\n" + right,
	})
	args = append([]string{"--min-lines", "0", "--min-nodes", "0"}, args...)
	code, stdout, stderr := runGodry(t, dir, args...)
	if code == 2 {
		t.Fatalf("godry failed: %s", stderr)
	}
	return code, stdout
}

func sameStructure(t *testing.T, left, right string) bool {
	t.Helper()
	code, _ := compareFuncs(t, left, right, "--threshold", "1")
	return code == 1
}

type funcPair struct {
	name, left, right string
}

func wantSameStructure(t *testing.T, pairs []funcPair) {
	t.Helper()
	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			if !sameStructure(t, pair.left, pair.right) {
				t.Errorf("score is below 1:\n%s\n%s", pair.left, pair.right)
			}
		})
	}
}

func wantDifferentStructure(t *testing.T, pairs []funcPair) {
	t.Helper()
	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			if sameStructure(t, pair.left, pair.right) {
				t.Errorf("score is 1:\n%s\n%s", pair.left, pair.right)
			}
		})
	}
}

func statements(name, left, right string) funcPair {
	return funcPair{
		name,
		"func alpha(a, b int) {\n\t" + left + "\n}\n",
		"func beta(a, b int) {\n\t" + right + "\n}\n",
	}
}

func TestRenamedNamesDoNotChangeTheScore(t *testing.T) {
	wantSameStructure(t, []funcPair{
		{
			"dryer sample with different receivers",
			"func (b *Board) alpha(xs []int) []int {\n\tys := filter(xs, odd)\n\treturn mapFn(ys, inc)\n}\n",
			"func (p *Panel) beta(items []int) []int {\n\tkept := filter(items, even)\n\treturn mapFn(kept, dec)\n}\n",
		},
		{
			"parameters and named results",
			"func alpha(a, b int) (sum int, err error) {\n\tsum = a\n\treturn\n}\n",
			"func beta(x, y int) (total int, failure error) {\n\ttotal = x\n\treturn\n}\n",
		},
		{
			"loop variables",
			"func alpha(xs []int) {\n\tfor i, x := range xs {\n\t\tuse(i, x)\n\t}\n}\n",
			"func beta(rows []int) {\n\tfor n, row := range rows {\n\t\tuse(n, row)\n\t}\n}\n",
		},
		{
			"labels",
			"func alpha() {\nouter:\n\tfor {\n\t\tbreak outer\n\t}\n}\n",
			"func beta() {\ndone:\n\tfor {\n\t\tbreak done\n\t}\n}\n",
		},
		{
			"package-level names outside call position",
			"func alpha() int {\n\treturn limit + offset\n}\n",
			"func beta() int {\n\treturn ceiling + margin\n}\n",
		},
	})
}

func TestChangedLiteralsDoNotChangeTheScore(t *testing.T) {
	literals := []string{`"text"`, "`raw`", "1", "0x10", "1.5", "2i", "'c'", "nil", "true", "false"}
	var pairs []funcPair
	for _, literal := range literals[1:] {
		pairs = append(pairs, statements(
			literals[0]+" and "+literal,
			"use(a, "+literals[0]+")",
			"use(a, "+literal+")",
		))
	}
	wantSameStructure(t, pairs)
}

func TestChangedFieldNamesDoNotChangeTheScore(t *testing.T) {
	wantSameStructure(t, []funcPair{
		{
			"selector",
			"func alpha(u User) string {\n\treturn u.Name\n}\n",
			"func beta(a Account) string {\n\treturn a.Email\n}\n",
		},
		{
			"assignment target",
			"func alpha(u *User) {\n\tu.Name = u.Alias\n}\n",
			"func beta(a *Account) {\n\ta.Email = a.Backup\n}\n",
		},
		{
			"composite literal key",
			"func alpha() Config {\n\treturn Config{Host: host, Port: 1}\n}\n",
			"func beta() Options {\n\treturn Options{Path: path, Mode: 2}\n}\n",
		},
		{
			"local struct type and tag",
			"func alpha() {\n\ttype row struct {\n\t\tID int `json:\"id\"`\n\t}\n}\n",
			"func beta() {\n\ttype item struct {\n\t\tKey string `json:\"key\"`\n\t}\n}\n",
		},
	})
}

func TestChangedTypeNamesOutsideCallPositionDoNotChangeTheScore(t *testing.T) {
	wantSameStructure(t, []funcPair{
		{
			"parameter and result",
			"func alpha(x int) string {\n\treturn use(x)\n}\n",
			"func beta(x float64) error {\n\treturn use(x)\n}\n",
		},
		statements("variable declaration", "var x int\n\tuse(x)", "var x Thing\n\tuse(x)"),
		statements("argument to make",
			"use(make([]int, 0), make(map[string]bool))",
			"use(make([]Row, 0), make(map[Key]Value))"),
		statements("composite literal", "use([]int{1}, Point{})", `use([]string{"a"}, Vector{})`),
		statements("type assertion", "use(a.(int))", "use(a.(Thing))"),
		statements("type argument of a call", "use[int](a)", "use[Thing](a)"),
		{
			"type parameter",
			"func alpha[T any](x T) {\n\tuse(x)\n}\n",
			"func beta[V comparable](x V) {\n\tuse(x)\n}\n",
		},
	})
}

func TestReceiverOfAMethodCallIsNormalized(t *testing.T) {
	wantSameStructure(t, []funcPair{
		{
			"package qualifier",
			"func alpha(xs []int) {\n\tfmt.Println(xs)\n}\n",
			"func beta(items []int) {\n\tlog.Println(items)\n}\n",
		},
		{
			"variable",
			"func alpha(w Writer) {\n\tw.Flush()\n}\n",
			"func beta(out Writer) {\n\tout.Flush()\n}\n",
		},
		{
			"field chain",
			"func alpha(s *Server) {\n\ts.conn.Close()\n}\n",
			"func beta(c *Client) {\n\tc.socket.Close()\n}\n",
		},
	})
}

func TestChangedCallsChangeTheScore(t *testing.T) {
	wantDifferentStructure(t, []funcPair{
		statements("function", "use(filter(a, odd))", "use(choose(a, odd))"),
		{
			"method",
			"func alpha(xs List) int {\n\treturn xs.Filter(odd)\n}\n",
			"func beta(items List) int {\n\treturn items.Choose(even)\n}\n",
		},
		statements("method earlier in a chain", "a.Where(b).Run()", "a.Limit(b).Run()"),
		statements("builtin", "use(len(a))", "use(cap(a))"),
		statements("conversion", "use(int64(a))", "use(float64(a))"),
		statements("generic function", "Keep[int](a)", "Drop[int](a)"),
		statements("generic function with two type arguments", "Keep[int, bool](a)", "Drop[int, bool](a)"),
		statements("call inside a function literal",
			"defer func() {\n\t\topen()\n\t}()",
			"defer func() {\n\t\tclose()\n\t}()"),
	})
}

func TestChangedOperatorsChangeTheScore(t *testing.T) {
	wantDifferentStructure(t, []funcPair{
		statements("arithmetic", "use(a + b)", "use(a - b)"),
		statements("comparison", "use(a == b)", "use(a != b)"),
		statements("logic", "use(a && b)", "use(a || b)"),
		statements("unary", "use(!a)", "use(-a)"),
		statements("assignment", "a += b", "a -= b"),
		statements("declaration or assignment", "a := b", "a = b"),
		statements("increment", "a++", "a--"),
		statements("branch", "for {\n\t\tbreak\n\t}", "for {\n\t\tcontinue\n\t}"),
		statements("address or dereference", "use(&a)", "use(*a)"),
		statements("var or const", "var a = b", "const a = b"),
	})
}

func TestDifferentShapeChangesTheScore(t *testing.T) {
	wantDifferentStructure(t, []funcPair{
		statements("extra statement",
			"c := filter(a, odd)\n\treturn mapFn(c, inc)",
			"c := filter(a, odd)\n\td := filter(c, odd)\n\treturn mapFn(d, inc)"),
		statements("if or for", "if a {\n\t\tuse()\n\t}", "for a {\n\t\tuse()\n\t}"),
		statements("name or literal", "use(a)", "use(1)"),
	})
}

// workFunc makes several calls, so that a single change leaves most of its structure in place.
func workFunc(name, load, check, save string) string {
	return fmt.Sprintf(`func %s(path string, limit int) error {
	data, err := %s(path)
	if err != nil {
		return err
	}
	if len(data) > limit {
		return errTooLarge
	}
	result, err := %s(data)
	if err != nil {
		return err
	}
	return %s(path, result)
}
`, name, load, check, save)
}

func TestFunctionsThatDoDifferentWorkDoNotMatchAtTheDefaultThreshold(t *testing.T) {
	code, stdout := compareFuncs(t,
		workFunc("alpha", "readFile", "validate", "writeFile"),
		workFunc("beta", "fetchURL", "decode", "publish"))

	if code != 0 || stdout != "No duplicates found.\n" {
		t.Errorf("exit code = %d, stdout = %q", code, stdout)
	}
}

func TestNearDuplicateIsReportedBelowAnExactMatch(t *testing.T) {
	left := workFunc("alpha", "readFile", "validate", "writeFile")
	near := strings.Replace(workFunc("beta", "readFile", "validate", "writeFile"),
		"\treturn writeFile(", "\tlog(path)\n\treturn writeFile(", 1)

	code, stdout := compareFuncs(t, left, near)

	if code != 1 || !strings.HasPrefix(stdout, "left.go:3: duplicates right.go:3 (score 0.") {
		t.Errorf("exit code = %d, stdout = %q", code, stdout)
	}
	if sameStructure(t, left, near) {
		t.Error("score is 1, want a score below 1")
	}
}

func TestScoreEqualToTheThresholdIsReported(t *testing.T) {
	copied := sumFunc("Sum")

	code, stdout := compareFuncs(t, copied, copied, "--threshold", "1")

	if want := "left.go:3: duplicates right.go:3 (score 1.00, literals 1.00, anchors 0)\n"; code != 1 || stdout != want {
		t.Errorf("exit code = %d, stdout = %q, want %q", code, stdout, want)
	}
}

func TestFunctionLiteralIsPartOfTheEnclosingFunction(t *testing.T) {
	wantSameStructure(t, []funcPair{{
		"renamed inside the literal",
		"func alpha(xs []int) {\n\tapply(xs, func(x int) int {\n\t\treturn twice(x, 1)\n\t})\n}\n",
		"func beta(rows []int) {\n\tapply(rows, func(row int) int {\n\t\treturn twice(row, 2)\n\t})\n}\n",
	}})
	wantDifferentStructure(t, []funcPair{{
		"different work inside the literal",
		"func alpha(xs []int) {\n\tapply(xs, func(x int) int {\n\t\treturn twice(x)\n\t})\n}\n",
		"func beta(xs []int) {\n\tapply(xs, func(x int) int {\n\t\treturn thrice(x)\n\t})\n}\n",
	}})
}

func TestFunctionLiteralGetsNoFindingOfItsOwn(t *testing.T) {
	literal := strings.TrimSpace(strings.Replace(sumFunc("x"), "func x(", "func(", 1))
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\nfunc One() {\n\tapply(" + literal + ")\n\tfirst()\n\tsecond(1, 2, 3)\n}\n",
		"b.go": "package a\n\nfunc Two(x, y string) (string, error) {\n\tif x == y {\n\t\treturn \"\", nil\n\t}\n" +
			"\tvisit(" + literal + ", x, y)\n\treturn parse(x + y)\n}\n",
		"c.go": "package a\n\nvar Three = " + literal,
	})

	code, stdout, _ := runGodry(t, dir)

	if code != 0 || stdout != "No duplicates found.\n" {
		t.Errorf("exit code = %d, stdout = %q", code, stdout)
	}
}

func TestFunctionWithNoBodyIsNotACandidate(t *testing.T) {
	code, stdout := compareFuncs(t, "func alpha(xs []int)\n", "func beta(xs []int)\n", "--threshold", "0")

	if code != 0 || stdout != "No duplicates found.\n" {
		t.Errorf("exit code = %d, stdout = %q", code, stdout)
	}
}

func TestFunctionWithNoBodyDoesNotHideItsNeighbour(t *testing.T) {
	declared := "func declared(xs []int)\n\n"

	_, stdout := compareFuncs(t, declared+sumFunc("One"), sumFunc("Two"))

	if want := "left.go:5: duplicates right.go:3 (score 1.00, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestMethodsAreComparedWithFunctions(t *testing.T) {
	_, stdout := compareFuncs(t, sumFunc("(a Adder) Sum"), sumFunc("Total"))

	if want := "left.go:3: duplicates right.go:3 (score 1.00, literals 1.00, anchors 0)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestSpanRunsFromFuncToTheClosingBrace(t *testing.T) {
	wrapped := func(name string) string {
		return "// " + name + " has a doc comment.\nfunc " + name + "(\n\ta int,\n\tb int,\n) int {\n" +
			"\tuse(a, b)\n\tuse(b, a)\n\tuse(a, a)\n\treturn a\n}\n"
	}
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\n" + wrapped("One"),
		"b.go": "package b\n\n" + wrapped("Two"),
	})

	_, stdout, _ := runGodry(t, dir, "--min-nodes", "0", "--min-lines", "9")
	if want := "a.go:4: duplicates b.go:4 (score 1.00, literals -, anchors 1)\n"; stdout != want {
		t.Errorf("--min-lines 9: stdout = %q, want %q", stdout, want)
	}

	_, stdout, _ = runGodry(t, dir, "--min-nodes", "0", "--min-lines", "10")
	if want := "No duplicates found.\n"; stdout != want {
		t.Errorf("--min-lines 10: stdout = %q, want %q", stdout, want)
	}
}

func TestCommentsDoNotChangeTheScore(t *testing.T) {
	wantSameStructure(t, []funcPair{
		{
			"doc comment",
			"// alpha uses x.\n// It uses it twice.\nfunc alpha(x int) {\n\tuse(x)\n}\n",
			"func beta(x int) {\n\tuse(x)\n}\n",
		},
		{
			"comment on a local declaration",
			"func alpha() {\n\t// Doc.\n\tconst x = 1 // Line.\n\tuse(x)\n}\n",
			"func beta() {\n\tconst x = 1\n\tuse(x)\n}\n",
		},
		{
			"comment on a local struct field",
			"func alpha() {\n\ttype row struct {\n\t\t// Doc.\n\t\tID int // Line.\n\t}\n}\n",
			"func beta() {\n\ttype row struct {\n\t\tID int\n\t}\n}\n",
		},
		{
			"comment in a signature",
			"func alpha(\n\t// Doc.\n\tx int, // Line.\n) {\n\tuse(x)\n}\n",
			"func beta(x int) {\n\tuse(x)\n}\n",
		},
	})
}

func TestParenthesesDoNotChangeTheScore(t *testing.T) {
	wantSameStructure(t, []funcPair{
		statements("around a called name", "use((len)(a))", "use(len(a))"),
		statements("around a method call", "(a.Run)()", "a.Run()"),
		statements("around an operand", "use((a))", "use(a)"),
	})
}

func TestGroupingStillChangesTheScore(t *testing.T) {
	wantDifferentStructure(t, []funcPair{
		statements("operand order", "use((a + b) * 2)", "use(a + b * 2)"),
	})
}

func TestDetailsOutsideTheChildNodesChangeTheScore(t *testing.T) {
	wantDifferentStructure(t, []funcPair{
		statements("spread argument", "use(a...)", "use(a)"),
		statements("slice from or slice to", "use(a[1:])", "use(a[:1])"),
		statements("loop init or post", "for a = b; ; {\n\t\tuse()\n\t}", "for ; ; a = b {\n\t\tuse()\n\t}"),
		statements("send or receive channel", "var c chan<- int\n\tuse(c)", "var c <-chan int\n\tuse(c)"),
		statements("one-way or two-way channel", "var c chan<- int\n\tuse(c)", "var c chan int\n\tuse(c)"),
		statements("alias or defined type", "type c = int\n\tuse(c)", "type c int\n\tuse(c)"),
		statements("declared type or initial value", "var c d\n\tuse(c)", "var c = d\n\tuse(c)"),
	})
}

func TestCallInTheSignatureKeepsItsName(t *testing.T) {
	wantDifferentStructure(t, []funcPair{{
		"array length",
		"func alpha(x [unsafe.Sizeof(a)]int) {\n\tuse(x)\n}\n",
		"func beta(x [unsafe.Alignof(a)]int) {\n\tuse(x)\n}\n",
	}})
}

func TestLineDirectiveDoesNotMoveTheSpan(t *testing.T) {
	body := strings.Repeat("\tuse(a)\n", 7) + "//line other.go:1\n" + "\tuse(a)\n"
	dir := writeTree(t, map[string]string{
		"a.go": "package a\n\nfunc One(a int) {\n" + body + "}\n",
		"b.go": "package b\n\n//line other.go:900\nfunc Two(a int) {\n" + body + "}\n",
	})

	_, stdout, _ := runGodry(t, dir, "--min-lines", "11", "--min-nodes", "0")

	if want := "a.go:3: duplicates b.go:4 (score 1.00, literals -, anchors 1)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestDeepExpressionIsRead(t *testing.T) {
	deep := "func %s() int {\n\treturn " + strings.Repeat("1 + ", 1200) + "1\n}\n"

	code, stdout := compareFuncs(t, fmt.Sprintf(deep, "alpha"), fmt.Sprintf(deep, "beta"))

	if want := "left.go:3: duplicates right.go:3 (score 1.00, literals -, anchors 0)\n"; code != 1 || stdout != want {
		t.Errorf("exit code = %d, stdout = %q", code, stdout)
	}
}

package goalie_test

import (
	"encoding/json"
	"testing"
)

var onlyPrimitiveArguments = `{"rules": {` + rulesOff("primitive-arguments") + `}}`

// onlyPrimitiveArgumentsAtZero makes one passed-down parameter report, so that a test of one parameter can fail.
var onlyPrimitiveArgumentsAtZero = `{"rules": {` + rulesOff("primitive-arguments") + `, "primitive-arguments": {"limit": 0}}}`

// repoCode gives the functions under test a method in the repo's own code to pass values down to.
const repoCode = `
type User struct{}

type Repo struct{}

func (Repo) Find(ctx context.Context, id string) (User, error) { return User{}, nil }

type Service struct{ repo Repo }
`

// checkPrimitives puts fn at line 5 of a.go. goalie doesn't type-check, so an unused import is fine.
func checkPrimitives(t *testing.T, settings, fn string) string {
	t.Helper()
	imports := `import ("context"; "fmt"; "os"; "strings"; "slices"; "math/rand/v2"; "gopkg.in/yaml.v3"; ` +
		`"github.com/mattn/go-sqlite3"; "github.com/opentracing/opentracing-go")`
	dir := repoWithNewCode(t, settings, imports+"\n\n"+fn+repoCode)
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	return stdout
}

const passesID = "func (s *Service) Get(ctx context.Context, id string) (User, error) {\n\treturn s.repo.Find(ctx, id)\n}\n"

func TestPrimitiveArgumentsReportsAParameterOnlyPassedDown(t *testing.T) {
	want := "a.go:5: warn primitive-arguments: Service.Get passes id down as string (1, limit 0)\n"
	if stdout := checkPrimitives(t, onlyPrimitiveArgumentsAtZero, passesID); stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestPrimitiveArgumentsNamesEachParameterOnlyPassedDown(t *testing.T) {
	// The parameter repo has the name of a field, and s.repo is no use of it.
	fn := "func (s *Service) Get(ctx context.Context, id string, page int, all bool, repo int) (User, error) {\n" +
		"\t_ = all\n\ts.repo.Find(ctx, fmt.Sprint(page))\n\tgo s.repo.Find(ctx, repo)\n\treturn s.repo.Find(ctx, id)\n}\n"
	want := "a.go:5: warn primitive-arguments: Service.Get passes id down as string, repo down as int (2, limit 1)\n"
	if stdout := checkPrimitives(t, onlyPrimitiveArguments, fn); stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestPrimitiveArgumentsSkipsAParameterThatTheFunctionUses(t *testing.T) {
	cases := []struct {
		name string
		fn   string
	}{
		{"used in an expression", "func greet(name string) string {\n\treturn \"Hello, \" + strings.TrimSpace(name)\n}\n"},
		{"passed into an imported package", "func read(path string) ([]byte, error) {\n\treturn os.ReadFile(path)\n}\n"},
		{"passed into a package with a major version", "func pick(n int) int {\n\treturn rand.N(n)\n}\n"},
		{"passed into a package with a version in its name", "func encode(name string) ([]byte, error) {\n" +
			"\treturn yaml.Marshal(name)\n}\n"},
		{"passed into a package with go- in its path", "func open(name string) error {\n" +
			"\treturn sqlite3.Open(name)\n}\n"},
		{"passed into a package with -go in its path", "func trace(name string) {\n\topentracing.StartSpan(name)\n}\n"},
		{"passed into a generic function of a package", "func find(names []string, name string) int {\n" +
			"\treturn slices.Index[[]string](names, name)\n}\n"},
		{"converted to a slice", "func save(path, content string) error {\n" +
			"\treturn os.WriteFile(path, []byte(content), 0o644)\n}\n"},
		{"converted to any", "func (s *Service) Get(ctx context.Context, id string) (User, error) {\n" +
			"\treturn s.repo.Find(ctx, fmt.Sprint(any(id)))\n}\n"},
		{"passed into len", "func valid(id string) bool {\n\treturn len(id) > 0\n}\n"},
		{"converted to a basic type", "func (s *Service) Get(ctx context.Context, n int) (User, error) {\n" +
			"\treturn s.repo.Find(ctx, string(rune(n)))\n}\n"},
		{"a defined type", "func (s *Service) Get(ctx context.Context, id UserID) (User, error) {\n" +
			"\treturn s.repo.Find(ctx, string(id))\n}\n\ntype UserID string\n"},
		{"never used", "func (s *Service) Get(ctx context.Context, id string) (User, error) {\n\treturn User{}, nil\n}\n"},
		{"passed down and used", "func (s *Service) Get(ctx context.Context, id string) (User, error) {\n" +
			"\tif id == \"\" {\n\t\treturn User{}, nil\n\t}\n\treturn s.repo.Find(ctx, id)\n}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if stdout := checkPrimitives(t, onlyPrimitiveArgumentsAtZero, tc.fn); stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
		})
	}
}

func TestPrimitiveArgumentsByDefaultAllowsOneParameter(t *testing.T) {
	if stdout := checkPrimitives(t, onlyPrimitiveArguments, passesID); stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
}

func TestPrimitiveArgumentsListsTheParametersInJSON(t *testing.T) {
	dir := repoWithNewCode(t, onlyPrimitiveArgumentsAtZero, "import \"context\"\n\n"+passesID+repoCode)
	code, stdout, stderr := runGoalie(t, dir, "check", "--base", "HEAD", "--format", "json")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	var output struct {
		Findings []struct {
			Parameters []struct{ Name, Type string } `json:"parameters"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil || len(output.Findings) != 1 {
		t.Fatalf("stdout = %s, want one finding; error: %v", stdout, err)
	}
	got := output.Findings[0].Parameters
	if len(got) != 1 || got[0].Name != "id" || got[0].Type != "string" {
		t.Errorf("parameters = %+v, want id of type string", got)
	}
}

package goalie

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/n-valchev/goalie/internal/check"
	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/gitchange"
	"github.com/n-valchev/goalie/internal/mcp"
)

// maxResults keeps an answer short enough for an agent to act on. The agent fixes these and calls
// again, and a cursor would point into a list that each edit changes.
const maxResults = 10

func runServe(args []string, dir string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 1 && slices.Contains(helpFlags, args[0]) {
		fmt.Fprint(stdout, usage)
		return exitClean
	}
	if len(args) > 0 {
		return fail(stderr, fmt.Errorf("serve takes no arguments, got %q", args[0]))
	}
	_, shared, _, err := repository(dir)
	if err != nil {
		return fail(stderr, fmt.Errorf("serve needs a Git repository: %w", err))
	}
	tools := served{shared: shared, stderr: stderr}
	server := mcp.Server{Name: "goalie", Version: version(), Tools: []mcp.Tool{tools.similarTool(), tools.checkTool(), tools.rulesTool()}}
	if err := server.Serve(stdin, stdout); err != nil {
		return fail(stderr, err)
	}
	return exitClean
}

func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "devel"
	}
	return info.Main.Version
}

// served holds what every tool shares. The server keeps no other state, so an answer can't be stale.
type served struct {
	shared string // The Git folder that every worktree of the server's repo shares.
	stderr io.Writer
}

// repository resolves links first, because a temp folder on macOS is a link, and Git names the
// folder that it points to.
func repository(dir string) (resolved, shared, top string, err error) {
	if resolved, err = filepath.EvalSymlinks(dir); err != nil {
		return "", "", "", err
	}
	if shared, top, err = gitchange.Repository(resolved); err != nil {
		return "", "", "", err
	}
	if shared, err = filepath.EvalSymlinks(shared); err != nil {
		return "", "", "", err
	}
	top, err = filepath.EvalSymlinks(top)
	return resolved, shared, top, err
}

// runDir takes dir from the input, and not the server's start folder, because a session can move
// into a worktree after the server starts. It walks up to the nearest .goalierc, so that each
// service in a monorepo gets its own limits.
func (s served) runDir(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("needs dir: the absolute path of the folder that you work in")
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("dir must be an absolute path, got %q", dir)
	}
	current, shared, top, err := repository(dir)
	if err != nil {
		return "", fmt.Errorf("dir %s is not in a Git repository: %w", dir, err)
	}
	if shared != s.shared {
		return "", fmt.Errorf("dir %s is in another repository than the one that goalie serve started in", dir)
	}
	for current != top {
		if _, err := os.Stat(filepath.Join(current, config.Name)); err == nil {
			return current, nil
		}
		current = filepath.Dir(current)
	}
	return top, nil
}

func decodeInput(arguments json.RawMessage, input any) error {
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		return fmt.Errorf("wrong input: %w", err)
	}
	return nil
}

func (s served) checkTool() mcp.Tool {
	return mcp.Tool{
		Name: "check",
		Description: "Check Go code against goalie's rules, with no test run. Gives up to 10 results, the worst first, " +
			"with more: the count left out. Each result has hint: one line on how to fix it. With scope changes, it checks what changed since base, uncommitted work included. " +
			"Then each result has change: new or worse when your edit made it, unchanged when it was there before, " +
			"and results that are new or worse come before the worst unchanged ones.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"dir"},
			"properties": map[string]any{
				"dir":   map[string]any{"type": "string", "description": "The absolute path of the folder that you work in."},
				"scope": map[string]any{"type": "string", "enum": []string{"changes", "all"}, "description": "changes (the default) or all."},
				"path":  map[string]any{"type": "string", "description": "Check only this package folder or file, relative to dir."},
				"base":  map[string]any{"type": "string", "description": "The revision to compare against. The default is base in .goalierc, then HEAD."},
			},
		},
		Call: s.check,
	}
}

type checkInput struct {
	Dir   string `json:"dir"`
	Scope string `json:"scope"`
	Path  string `json:"path"`
	Base  string `json:"base"`
}

type checkAnswer struct {
	Results  []check.Result `json:"results"`
	More     int            `json:"more"`
	Base     string         `json:"base,omitempty"`
	Unparsed []string       `json:"unparsed"`
}

func (s served) check(arguments json.RawMessage) (string, error) {
	var input checkInput
	if err := decodeInput(arguments, &input); err != nil {
		return "", err
	}
	runDir, err := s.runDir(input.Dir)
	if err != nil {
		return "", err
	}
	file, err := config.Read(runDir)
	if err != nil {
		return "", err
	}
	opts := options{partial: true}
	switch input.Scope {
	case "", string(config.ScopeChanges):
	case string(config.ScopeAll):
		opts.all = true
	default:
		return "", fmt.Errorf("scope must be changes or all, got %q", input.Scope)
	}
	// path replaces paths from .goalierc, so that the agent can look at any one package.
	if input.Path != "" {
		if file.Paths, err = pathIn(input.Dir, runDir, input.Path); err != nil {
			return "", err
		}
	}
	// HEAD and not an error, because an agent with uncommitted work needs an answer. CI doesn't use
	// the server, so its guard against a forgotten revision still holds.
	opts.base = cmp.Or(input.Base, file.Base, "HEAD")
	run := newRun()
	run.Parser.SkipUnparsed()
	selected, err := selectRules(file, opts, run)
	if err != nil {
		return "", err
	}
	var log bytes.Buffer
	results, failures := runRules(selected, runDir, &log)
	// Any failure, and not only a failure of every rule as in goalie check, because an agent would
	// read a short list with no error as clean code.
	if failures > 0 {
		return "", errors.New(strings.TrimSpace(log.String()))
	}
	s.stderr.Write(log.Bytes())
	if !opts.all {
		check.Mark(results, s.baseResults(file, opts, run, runDir, results))
	}
	check.SortResults(results)
	check.AddHints(results)
	shown := append([]check.Result{}, results[:min(maxResults, len(results))]...)
	answer := checkAnswer{Results: shown, More: len(results) - len(shown), Unparsed: run.Parser.Unparsed()}
	if !opts.all {
		answer.Base = opts.base
	}
	encoded, err := json.Marshal(answer)
	return string(encoded), err
}

// baseResults runs the same rules on the code at the base. An error leaves every result new instead
// of failing the call, because the agent still needs the results.
func (s served) baseResults(file config.Settings, opts options, run check.Scope, runDir string, results []check.Result) []check.Result {
	if len(results) == 0 {
		return nil
	}
	if err := run.Repo.Apply(file, nil); err != nil {
		fail(s.stderr, err)
		return nil
	}
	baseRun, err := run.AtBase(runDir, opts.base, results)
	if err != nil {
		fail(s.stderr, fmt.Errorf("can't read the code at %s: %w", opts.base, err))
		return nil
	}
	// Every function in the listed folders, because the changed lines are numbered as they are now, and
	// not as they were at the base.
	opts.all = true
	selected, err := selectRules(file, opts, baseRun)
	if err != nil {
		fail(s.stderr, err)
		return nil
	}
	var log bytes.Buffer
	base, failures := runRules(selected, runDir, &log)
	// Only the errors, because the notices of the base run would read as notices of the check.
	if failures > 0 {
		s.stderr.Write(log.Bytes())
	}
	return base
}

// pathIn turns path, relative to dir, into a path relative to the run folder, where the rules
// look for files.
func pathIn(dir, runDir, path string) ([]string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("path %s: %w", path, err)
	}
	rel, err := filepath.Rel(runDir, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path %s is outside %s, the folder of the nearest %s", path, runDir, config.Name)
	}
	return []string{rel}, nil
}

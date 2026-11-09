package goalie_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/n-valchev/goalie/internal/goalie"
)

const modernVersion = "2026-07-28"

type rpcResponse struct {
	ID     any             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	} `json:"error"`
}

type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

type checkAnswer struct {
	Results  []map[string]any `json:"results"`
	More     int              `json:"more"`
	Base     string           `json:"base"`
	Unparsed []string         `json:"unparsed"`
}

type lockedBuffer struct {
	lock sync.Mutex
	buf  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.buf.String()
}

// mcpClient talks to a running goalie serve through pipes, so that a test can change files between
// two calls, as an agent does.
type mcpClient struct {
	t      *testing.T
	in     *io.PipeWriter
	lines  chan string
	stderr *lockedBuffer
	nextID int
}

func startServe(t *testing.T, dir string) *mcpClient {
	t.Helper()
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	client := &mcpClient{t: t, in: inWriter, lines: make(chan string), stderr: &lockedBuffer{}}
	exit := make(chan int, 1)
	go func() {
		exit <- goalie.Run([]string{"serve"}, dir, inReader, outWriter, client.stderr)
		outWriter.Close()
	}()
	go func() {
		scanner := bufio.NewScanner(outReader)
		scanner.Buffer(nil, 1<<24)
		for scanner.Scan() {
			client.lines <- scanner.Text()
		}
		close(client.lines)
	}()
	t.Cleanup(func() {
		inWriter.Close()
		select {
		case code := <-exit:
			if code != 0 {
				t.Errorf("exit code = %d, want 0; stderr: %s", code, client.stderr)
			}
		case <-time.After(10 * time.Second):
			t.Error("the server didn't stop when stdin closed")
		}
	})
	return client
}

func (c *mcpClient) send(line string) rpcResponse {
	c.t.Helper()
	if _, err := io.WriteString(c.in, line+"\n"); err != nil {
		c.t.Fatal(err)
	}
	select {
	case got, ok := <-c.lines:
		if !ok {
			c.t.Fatalf("the server stopped; stderr: %s", c.stderr)
		}
		var response rpcResponse
		if err := json.Unmarshal([]byte(got), &response); err != nil {
			c.t.Fatalf("response %q: %v", got, err)
		}
		return response
	case <-time.After(30 * time.Second):
		c.t.Fatalf("no response to %s; stderr: %s", line, c.stderr)
	}
	return rpcResponse{}
}

func (c *mcpClient) request(method string, params any) rpcResponse {
	c.t.Helper()
	c.nextID++
	line, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.nextID, "method": method, "params": params})
	if err != nil {
		c.t.Fatal(err)
	}
	response := c.send(string(line))
	if response.ID != float64(c.nextID) {
		c.t.Errorf("id = %v, want %d", response.ID, c.nextID)
	}
	return response
}

func (c *mcpClient) initialize() {
	c.t.Helper()
	response := c.request("initialize", map[string]any{
		"protocolVersion": "2025-11-25", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "test", "version": "1"},
	})
	if response.Error != nil {
		c.t.Fatalf("initialize: %+v", response.Error)
	}
	if _, err := io.WriteString(c.in, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"); err != nil {
		c.t.Fatal(err)
	}
}

func (c *mcpClient) callTool(name string, args map[string]any) toolResult {
	c.t.Helper()
	response := c.request("tools/call", map[string]any{"name": name, "arguments": args})
	if response.Error != nil {
		c.t.Fatalf("tools/call %s: %+v", name, response.Error)
	}
	var result toolResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		c.t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		c.t.Fatalf("content = %+v, want one text block", result.Content)
	}
	return result
}

func (c *mcpClient) check(args map[string]any) checkAnswer {
	c.t.Helper()
	result := c.callTool("check", args)
	if result.IsError {
		c.t.Fatalf("check gave an error: %s", result.Content[0].Text)
	}
	var answer checkAnswer
	if err := json.Unmarshal([]byte(result.Content[0].Text), &answer); err != nil {
		c.t.Fatalf("answer %q: %v", result.Content[0].Text, err)
	}
	return answer
}

func (c *mcpClient) checkError(args map[string]any) string {
	c.t.Helper()
	result := c.callTool("check", args)
	if !result.IsError {
		c.t.Fatalf("check gave no error: %s", result.Content[0].Text)
	}
	return result.Content[0].Text
}

func toolNames(t *testing.T, response rpcResponse) []string {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("tools/list: %+v", response.Error)
	}
	var list struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(response.Result, &list); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range list.Tools {
		if tool.InputSchema["type"] != "object" {
			t.Errorf("%s: inputSchema = %v, want an object schema", tool.Name, tool.InputSchema)
		}
		names = append(names, tool.Name)
	}
	return names
}

func functionNames(answer checkAnswer) string {
	var names []string
	for _, result := range answer.Results {
		names = append(names, fmt.Sprint(result["function"]))
	}
	return strings.Join(names, ", ")
}

func longFunction(name string, lines int) string {
	return fmt.Sprintf("func %s() int {\n%s\treturn 0\n}\n", name, strings.Repeat("\t_ = 0\n", lines-3))
}

// serveRepo checks only long-function at a limit of 5, so that each test controls its results.
func serveRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	all := map[string]string{".goalierc": `{"rules": {` + rulesOff("long-function") + `, "long-function": {"limit": 5}}}`}
	for name, content := range files {
		all[name] = content
	}
	return gitRepo(t, all)
}

func TestServeAnswersACheckAfterInitialize(t *testing.T) {
	dir := serveRepo(t, map[string]string{"a.go": "package a\n\n" + longFunction("Short", 4)})
	write(t, dir, "a.go", "package a\n\n"+longFunction("Short", 4)+"\n"+longFunction("Long", 7))
	client := startServe(t, dir)

	response := client.request("initialize", map[string]any{
		"protocolVersion": "2025-11-25", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "test", "version": "1"},
	})
	var initialized struct {
		ProtocolVersion string `json:"protocolVersion"`
		Capabilities    struct {
			Tools map[string]any `json:"tools"`
		} `json:"capabilities"`
		ServerInfo struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(response.Result, &initialized); err != nil {
		t.Fatal(err)
	}
	if initialized.ProtocolVersion != "2025-11-25" || initialized.Capabilities.Tools == nil || initialized.ServerInfo.Name != "goalie" {
		t.Errorf("initialize = %s", response.Result)
	}

	if names := toolNames(t, client.request("tools/list", map[string]any{})); strings.Join(names, ",") != "find_similar,check,rules,risk" {
		t.Errorf("tools = %v, want only find_similar, check, rules, and risk", names)
	}

	answer := client.check(map[string]any{"dir": dir})
	if len(answer.Results) != 1 || answer.More != 0 || answer.Base != "HEAD" || len(answer.Unparsed) != 0 {
		t.Fatalf("answer = %+v, want one result against HEAD", answer)
	}
	wantFields(t, answer.Results[0], map[string]any{
		"rule": "long-function", "mode": "warn", "value": 7.0, "limit": 5.0, "file": "a.go", "function": "Long",
	})
}

func TestServeAnswersTheProtocolOf2026WithNoInitialize(t *testing.T) {
	dir := serveRepo(t, map[string]string{"a.go": "package a\n\n" + longFunction("Long", 7)})
	client := startServe(t, dir)
	meta := map[string]any{
		"io.modelcontextprotocol/protocolVersion":    modernVersion,
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}

	response := client.request("server/discover", map[string]any{"_meta": meta})
	var discovered struct {
		ResultType        string         `json:"resultType"`
		SupportedVersions []string       `json:"supportedVersions"`
		Capabilities      map[string]any `json:"capabilities"`
		CacheScope        string         `json:"cacheScope"`
	}
	if err := json.Unmarshal(response.Result, &discovered); err != nil {
		t.Fatal(err)
	}
	if discovered.ResultType != "complete" || strings.Join(discovered.SupportedVersions, ",") != modernVersion ||
		discovered.Capabilities["tools"] == nil || discovered.CacheScope != "public" {
		t.Errorf("server/discover = %s", response.Result)
	}

	if names := toolNames(t, client.request("tools/list", map[string]any{"_meta": meta})); strings.Join(names, ",") != "find_similar,check,rules,risk" {
		t.Errorf("tools = %v, want only find_similar, check, rules, and risk", names)
	}
	response = client.request("tools/call", map[string]any{"_meta": meta, "name": "check", "arguments": map[string]any{"dir": dir, "scope": "all"}})
	var result struct {
		ResultType string `json:"resultType"`
		toolResult
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.ResultType != "complete" || result.IsError || !strings.Contains(result.Content[0].Text, `"function":"Long"`) {
		t.Errorf("tools/call = %s", response.Result)
	}
}

func TestServeRejectsARequestOf2026ThatItCannotServe(t *testing.T) {
	client := startServe(t, serveRepo(t, nil))
	cases := []struct {
		name     string
		meta     map[string]any
		wantCode int
	}{
		{"no protocol version", map[string]any{}, -32602},
		{"an unknown protocol version", map[string]any{
			"io.modelcontextprotocol/protocolVersion":    "2099-01-01",
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		}, -32022},
		{"no client capabilities", map[string]any{"io.modelcontextprotocol/protocolVersion": modernVersion}, -32602},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := client.request("tools/list", map[string]any{"_meta": tc.meta})
			if response.Error == nil || response.Error.Code != tc.wantCode {
				t.Errorf("error = %+v, want code %d", response.Error, tc.wantCode)
			}
		})
	}
}

func TestServeAnswersAnOlderVersionThatTheClientAsksFor(t *testing.T) {
	client := startServe(t, serveRepo(t, nil))
	cases := map[string]string{"2025-06-18": "2025-06-18", "2024-01-01": "2025-11-25"}
	for asked, want := range cases {
		response := client.request("initialize", map[string]any{"protocolVersion": asked, "capabilities": map[string]any{}})
		var initialized struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(response.Result, &initialized); err != nil {
			t.Fatal(err)
		}
		if initialized.ProtocolVersion != want {
			t.Errorf("asked for %s, got %s, want %s", asked, initialized.ProtocolVersion, want)
		}
	}
}

func TestServeKeepsRunningAfterABadRequest(t *testing.T) {
	dir := serveRepo(t, map[string]string{"a.go": "package a\n\n" + longFunction("Long", 7)})
	client := startServe(t, dir)
	client.initialize()
	cases := []struct {
		name, line string
		wantCode   int
	}{
		{"a line that isn't JSON", `{"jsonrpc": "2.0", "id": 1,`, -32700},
		{"a request with no method", `{"jsonrpc": "2.0", "id": 1}`, -32600},
		{"a method that isn't a string", `{"jsonrpc": "2.0", "id": 1, "method": 3}`, -32600},
		{"a list of requests", `[{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}]`, -32600},
		{"an unknown method", `{"jsonrpc": "2.0", "id": 1, "method": "resources/list"}`, -32601},
		{"an unknown tool", `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "fix", "arguments": {}}}`, -32602},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := client.send(tc.line)
			if response.Error == nil || response.Error.Code != tc.wantCode {
				t.Errorf("error = %+v, want code %d", response.Error, tc.wantCode)
			}
		})
	}
	if answer := client.check(map[string]any{"dir": dir, "scope": "all"}); len(answer.Results) != 1 {
		t.Errorf("answer = %+v, want one result", answer)
	}
}

func TestServeGivesAToolErrorThatNamesTheWrongInput(t *testing.T) {
	dir := serveRepo(t, nil)
	client := startServe(t, dir)
	client.initialize()
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"no dir", map[string]any{}, "dir"},
		{"a relative dir", map[string]any{"dir": "."}, "dir"},
		{"an unknown scope", map[string]any{"dir": dir, "scope": "some"}, "scope"},
		{"an unknown input", map[string]any{"dir": dir, "limit": 3}, "limit"},
		{"a dir that isn't a string", map[string]any{"dir": 3}, "dir"},
		{"a path outside dir's repo", map[string]any{"dir": dir, "path": "../other"}, "path"},
		{"an unknown base", map[string]any{"dir": dir, "base": "no-such-branch"}, "no-such-branch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if text := client.checkError(tc.args); !strings.Contains(text, tc.want) {
				t.Errorf("error = %q, want it to name %s", text, tc.want)
			}
		})
	}
}

func TestServeAcceptsOnlyADirInItsRepositoryOrAWorktree(t *testing.T) {
	dir := serveRepo(t, map[string]string{"a.go": "package a\n\n" + longFunction("Long", 7)})
	other := serveRepo(t, map[string]string{"a.go": "package a\n\n" + longFunction("Long", 7)})
	worktree := filepath.Join(t.TempDir(), "wt")
	git(t, dir, "worktree", "add", "-b", "feature", worktree)
	write(t, worktree, "b.go", "package a\n\n"+longFunction("InWorktree", 8))
	client := startServe(t, dir)
	client.initialize()

	if text := client.checkError(map[string]any{"dir": other}); !strings.Contains(text, other) {
		t.Errorf("error = %q, want it to name the dir", text)
	}
	if text := client.checkError(map[string]any{"dir": t.TempDir()}); !strings.Contains(text, "repository") {
		t.Errorf("error = %q, want it to say the dir is in no repository", text)
	}
	answer := client.check(map[string]any{"dir": worktree})
	if len(answer.Results) != 1 || answer.Results[0]["function"] != "InWorktree" {
		t.Errorf("answer = %+v, want the change in the worktree", answer)
	}
}

func TestServeReadsTheGoaliercNearestToDir(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		".goalierc":     `{"rules": {` + rulesOff("long-function") + `, "long-function": {"limit": 5}}}`,
		"svc/.goalierc": `{"rules": {` + rulesOff("long-function") + `, "long-function": {"limit": 7}}}`,
		"svc/a.go":      "package a\n",
	})
	write(t, dir, "svc/a.go", "package a\n\n"+longFunction("Seven", 7)+"\n"+longFunction("Eight", 8))
	write(t, dir, "svc/deep/b.go", "package deep\n\n"+longFunction("Nine", 9))
	client := startServe(t, dir)
	client.initialize()

	answer := client.check(map[string]any{"dir": filepath.Join(dir, "svc", "deep")})
	var got []string
	for _, result := range answer.Results {
		got = append(got, fmt.Sprintf("%s %v", result["file"], result["function"]))
	}
	if strings.Join(got, ", ") != "deep/b.go Nine, a.go Eight" {
		t.Errorf("results = %v, want the limit 7 of svc/.goalierc, with paths from svc", got)
	}
}

func TestServeTakesTheRevisionFromTheInputThenGoaliercThenHEAD(t *testing.T) {
	settings := func(base string) string {
		return `{` + base + `"rules": {` + rulesOff("long-function") + `, "long-function": {"limit": 5}}}`
	}
	dir := gitRepo(t, map[string]string{".goalierc": settings(""), "a.go": "package a\n"})
	git(t, dir, "branch", "start")
	write(t, dir, "a.go", "package a\n\n"+longFunction("Committed", 7))
	git(t, dir, "commit", "--all", "--message", "second")
	git(t, dir, "branch", "second")
	write(t, dir, "b.go", "package a\n\n"+longFunction("Uncommitted", 7))
	client := startServe(t, dir)
	client.initialize()

	functions := func(answer checkAnswer) string { return answer.Base + ": " + functionNames(answer) }
	if got := functions(client.check(map[string]any{"dir": dir})); got != "HEAD: Uncommitted" {
		t.Errorf("with no base anywhere: %s", got)
	}
	if got := functions(client.check(map[string]any{"dir": dir, "base": "start"})); got != "start: Committed, Uncommitted" {
		t.Errorf("with the base input: %s", got)
	}
	write(t, dir, ".goalierc", settings(`"base": "start", `))
	if got := functions(client.check(map[string]any{"dir": dir})); got != "start: Committed, Uncommitted" {
		t.Errorf("with base in .goalierc: %s", got)
	}
	if got := functions(client.check(map[string]any{"dir": dir, "base": "second"})); got != "second: Uncommitted" {
		t.Errorf("with both, the input wins: %s", got)
	}
}

func TestServeSkipsAFileThatDoesNotParse(t *testing.T) {
	dir := serveRepo(t, map[string]string{"a.go": "package a\n", "b.go": "package a\n"})
	write(t, dir, "a.go", "package a\n\n"+longFunction("Long", 7))
	write(t, dir, "b.go", "package a\n\nfunc Half() {\n\tif {\n")
	client := startServe(t, dir)
	client.initialize()

	answer := client.check(map[string]any{"dir": dir})
	if len(answer.Results) != 1 || answer.Results[0]["function"] != "Long" || strings.Join(answer.Unparsed, ",") != "b.go" {
		t.Errorf("answer = %+v, want the result of a.go and b.go under unparsed", answer)
	}
	if code, _, _ := runGoalie(t, dir, "check", "--partial", "--base", "HEAD"); code != 2 {
		t.Errorf("goalie check exit code = %d, want 2", code)
	}
}

func TestServeGivesTheWorst10ResultsAndTheCountOfTheRest(t *testing.T) {
	var source strings.Builder
	source.WriteString("package a\n")
	for i := range 12 {
		source.WriteString("\n" + longFunction(fmt.Sprintf("F%02d", i), 6+i))
	}
	dir := serveRepo(t, map[string]string{"a.go": "package a\n"})
	write(t, dir, "a.go", source.String())
	client := startServe(t, dir)
	client.initialize()

	answer := client.check(map[string]any{"dir": dir})
	if len(answer.Results) != 10 || answer.More != 2 {
		t.Fatalf("got %d results and more %d, want 10 and 2", len(answer.Results), answer.More)
	}
	if answer.Results[0]["function"] != "F11" || answer.Results[9]["function"] != "F02" {
		t.Errorf("first %v, last %v, want F11 and F02", answer.Results[0]["function"], answer.Results[9]["function"])
	}
}

func TestServeNarrowsACheckToPath(t *testing.T) {
	dir := serveRepo(t, map[string]string{
		"a/a.go": "package a\n\n" + longFunction("InA", 7),
		"b/b.go": "package b\n\n" + longFunction("InB", 7),
		"b/c.go": "package b\n\n" + longFunction("InC", 7),
	})
	client := startServe(t, dir)
	client.initialize()
	cases := []struct{ dir, path, want string }{
		{dir, "b", "InB, InC"},
		{dir, "b/c.go", "InC"},
		{filepath.Join(dir, "b"), "c.go", "InC"},
		{dir, filepath.Join(dir, "a"), "InA"},
	}
	for _, tc := range cases {
		answer := client.check(map[string]any{"dir": tc.dir, "scope": "all", "path": tc.path})
		if got := functionNames(answer); got != tc.want {
			t.Errorf("dir %s, path %s: got %s, want %s", tc.dir, tc.path, got, tc.want)
		}
	}
}

func TestHelpListsServe(t *testing.T) {
	_, stdout, _ := runGoalie(t, t.TempDir(), "--help")
	if !strings.Contains(stdout, "goalie serve\n") || !strings.Contains(stdout, "Serve:\n") {
		t.Errorf("help = %q, want goalie serve in the usage and a Serve section", stdout)
	}
}

func TestServeReadsAFileThatChangedSinceTheLastCall(t *testing.T) {
	dir := serveRepo(t, map[string]string{"a.go": "package a\n"})
	write(t, dir, "a.go", "package a\n\n"+longFunction("Grows", 5))
	client := startServe(t, dir)
	client.initialize()

	if answer := client.check(map[string]any{"dir": dir}); len(answer.Results) != 0 {
		t.Fatalf("answer = %+v, want no result", answer)
	}
	write(t, dir, "a.go", "package a\n\n"+longFunction("Grows", 6))
	if answer := client.check(map[string]any{"dir": dir}); len(answer.Results) != 1 || answer.Results[0]["value"] != 6.0 {
		t.Errorf("answer = %+v, want the new result", answer)
	}
}

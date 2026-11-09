// Package mcp serves tools over the Model Context Protocol, on stdio with one JSON-RPC message for
// each line. It knows nothing about goalie.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

// ProtocolVersion is the protocol that a client of 2026 asks for in the _meta of each request. A client of
// 2025 opens with initialize instead, so the server speaks both: Claude Code may use either.
const ProtocolVersion = "2026-07-28"

// legacyVersions are the versions that initialize can agree on, the latest first.
var legacyVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

const (
	metaVersion      = "io.modelcontextprotocol/protocolVersion"
	metaCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaServerInfo   = "io.modelcontextprotocol/serverInfo"
)

const (
	codeParse              = -32700
	codeInvalidRequest     = -32600
	codeUnknownMethod      = -32601
	codeInvalidParams      = -32602
	codeUnsupportedVersion = -32022
)

// The tool list never changes while the server runs.
const listTTLMs = 3_600_000

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	// Call returns the text of the answer. An error becomes a result with isError, so that the agent
	// reads it and can try again.
	Call func(arguments json.RawMessage) (string, error)
}

type Server struct {
	Name, Version string
	Tools         []Tool
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *rpcError) Error() string { return e.Message }

// Serve answers each line of in until in ends. Requests run one at a time, in order.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	session := &session{server: s}
	reader := bufio.NewReader(in)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			if reply, ok := session.handle(line); ok {
				encoded, err := json.Marshal(reply)
				if err != nil {
					return err
				}
				if _, err := out.Write(append(encoded, '\n')); err != nil {
					return err
				}
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

// session holds whether the client opened with initialize, the only state of a stdio process.
type session struct {
	server *Server
	legacy bool
}

func (s *session) handle(line []byte) (response, bool) {
	null := json.RawMessage("null")
	if !json.Valid(line) {
		return failure(null, &rpcError{Code: codeParse, Message: "the line is not JSON"}), true
	}
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return failure(null, &rpcError{Code: codeInvalidRequest, Message: "a request must be one JSON object: " + err.Error()}), true
	}
	// A notification gets no answer, even one that the server doesn't know.
	if req.ID == nil {
		return response{}, false
	}
	if req.Method == "" || req.JSONRPC != "2.0" {
		return failure(req.ID, &rpcError{Code: codeInvalidRequest, Message: `a request needs "jsonrpc": "2.0" and a method`}), true
	}
	result, err := s.dispatch(req)
	if err != nil {
		var rpcErr *rpcError
		if !errors.As(err, &rpcErr) {
			rpcErr = &rpcError{Code: codeInvalidParams, Message: err.Error()}
		}
		return failure(req.ID, rpcErr), true
	}
	return response{JSONRPC: "2.0", ID: req.ID, Result: result}, true
}

func failure(id json.RawMessage, err *rpcError) response {
	return response{JSONRPC: "2.0", ID: id, Error: err}
}

type params struct {
	Meta            map[string]json.RawMessage `json:"_meta"`
	ProtocolVersion string                     `json:"protocolVersion"`
	Name            string                     `json:"name"`
	Arguments       json.RawMessage            `json:"arguments"`
}

func (s *session) dispatch(req request) (map[string]any, error) {
	var p params
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, fmt.Errorf("params: %w", err)
		}
	}
	switch req.Method {
	case "initialize":
		s.legacy = true
		return s.initialize(p.ProtocolVersion), nil
	case "ping":
		return map[string]any{}, nil
	}
	if !s.legacy {
		if err := checkMeta(p.Meta); err != nil {
			return nil, err
		}
	}
	var result map[string]any
	switch req.Method {
	case "server/discover":
		result = s.discover()
	case "tools/list":
		result = map[string]any{"tools": s.server.list(), "ttlMs": listTTLMs, "cacheScope": "public"}
	case "tools/call":
		call, err := s.server.call(p.Name, p.Arguments)
		if err != nil {
			return nil, err
		}
		result = call
	default:
		return nil, &rpcError{Code: codeUnknownMethod, Message: fmt.Sprintf("unknown method %q", req.Method)}
	}
	result["resultType"] = "complete"
	return result, nil
}

// checkMeta asks a client of 2026 for the fields that the protocol of 2026 requires in each request.
func checkMeta(meta map[string]json.RawMessage) error {
	raw, ok := meta[metaVersion]
	if !ok {
		return &rpcError{Code: codeInvalidParams, Message: fmt.Sprintf("needs _meta %q, or initialize first", metaVersion)}
	}
	var version string
	if json.Unmarshal(raw, &version) != nil || version != ProtocolVersion {
		return &rpcError{
			Code: codeUnsupportedVersion, Message: fmt.Sprintf("protocol version %s is not supported", raw),
			Data: map[string]any{"supported": []string{ProtocolVersion}, "requested": raw},
		}
	}
	if _, ok := meta[metaCapabilities]; !ok {
		return &rpcError{Code: codeInvalidParams, Message: fmt.Sprintf("needs _meta %q", metaCapabilities)}
	}
	return nil
}

func (s *session) info() map[string]string {
	return map[string]string{"name": s.server.Name, "version": s.server.Version}
}

func (s *session) initialize(asked string) map[string]any {
	version := legacyVersions[0]
	if slices.Contains(legacyVersions, asked) {
		version = asked
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      s.info(),
	}
}

func (s *session) discover() map[string]any {
	return map[string]any{
		"supportedVersions": []string{ProtocolVersion},
		"capabilities":      map[string]any{"tools": map[string]any{}},
		"_meta":             map[string]any{metaServerInfo: s.info()},
		"ttlMs":             listTTLMs,
		"cacheScope":        "public",
	}
}

func (s *Server) list() []map[string]any {
	tools := make([]map[string]any, 0, len(s.Tools))
	for _, tool := range s.Tools {
		tools = append(tools, map[string]any{"name": tool.Name, "description": tool.Description, "inputSchema": tool.InputSchema})
	}
	return tools
}

func (s *Server) call(name string, arguments json.RawMessage) (map[string]any, error) {
	index := slices.IndexFunc(s.Tools, func(tool Tool) bool { return tool.Name == name })
	if index < 0 {
		return nil, &rpcError{Code: codeInvalidParams, Message: fmt.Sprintf("unknown tool %q", name)}
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage("{}")
	}
	text, err := safeCall(s.Tools[index], arguments)
	isError := err != nil
	if isError {
		text = err.Error()
	}
	return map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "isError": isError}, nil
}

// safeCall turns a panic into a tool error, because one bad call must not stop the agent's server.
func safeCall(tool Tool, arguments json.RawMessage) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s failed: %v", tool.Name, r)
		}
	}()
	return tool.Call(arguments)
}

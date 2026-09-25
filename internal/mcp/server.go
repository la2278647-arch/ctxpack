// Package mcp implements a minimal Model Context Protocol server over stdio.
//
// It speaks JSON-RPC 2.0 with newline-delimited messages (the MCP stdio
// transport) and exposes five tools that any MCP-capable agent (Claude
// Desktop, Cursor, Codex, ...) can call:
//
//   - pack_repo(path, format?, include?, exclude?, max_size?, no_gitignore?,
//     hidden?, budget?) -> packed bundle text
//   - repo_map(path, include?, exclude?, ...) -> token-aware tree text
//   - count_tokens(path) -> total tokens + per-model fit
//   - list_models() -> the model table, with each model's effective limit
//   - diff_repo(path, ref?, format?, budget?) -> packed diff bundle text
//
// The implementation is stdlib-only and synchronous, which is enough for local
// single-client use.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/la2278647-arch/ctxpack/internal/counter"
	"github.com/la2278647-arch/ctxpack/internal/format"
	"github.com/la2278647-arch/ctxpack/internal/gitutil"
	"github.com/la2278647-arch/ctxpack/internal/packer"
	"github.com/la2278647-arch/ctxpack/internal/repomap"
	"github.com/la2278647-arch/ctxpack/internal/walker"
)

const protocolVersion = "2024-11-05"

// Serve runs the MCP server loop on the given reader/writer until EOF.
func Serve(r io.Reader, w io.Writer, version string) error {
	server := &server{w: w, version: version}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// Malformed input: respond with a parse error if we can.
			server.writeError(nil, -32700, "Parse error")
			continue
		}
		server.handle(msg)
	}
	return scanner.Err()
}

type server struct {
	w       io.Writer
	version string
	enc     json.Encoder
}

// rpcRequest is a JSON-RPC 2.0 request.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

func (s *server) handle(msg map[string]any) {
	method, _ := msg["method"].(string)
	id, hasID := msg["id"]
	params := msg["params"]

	switch method {
	case "initialize":
		s.writeResult(id, map[string]any{
			"protocolVersion": protocolVersion,
			"serverInfo": map[string]any{
				"name":    "ctxpack",
				"version": s.version,
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		})
	case "notifications/initialized":
		// No response expected for notifications.
	case "tools/list":
		s.writeResult(id, map[string]any{"tools": tools()})
	case "tools/call":
		s.handleToolCall(id, params)
	case "ping":
		s.writeResult(id, map[string]any{})
	default:
		if hasID {
			s.writeError(id, -32601, "Method not found: "+method)
		}
	}
}

func tools() []map[string]any {
	return []map[string]any{
		{
			"name":        "pack_repo",
			"description": "Pack a local repository or directory into a single LLM-optimized context document. Returns the packed bundle (XML by default) ready to paste into a chat with an LLM.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":         map[string]any{"type": "string", "description": "Absolute or relative path to the repository root."},
					"format":       map[string]any{"type": "string", "enum": []string{"xml", "markdown", "json", "text"}, "default": "xml"},
					"include":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Globs to include (e.g. \"*.go\")."},
					"exclude":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Globs to exclude."},
					"max_size":     map[string]any{"type": "integer", "description": "Read no more than N bytes of a file; larger files are still listed, without content."},
					"no_gitignore": map[string]any{"type": "boolean", "default": false},
					"hidden":       map[string]any{"type": "boolean", "default": false},
					"max_depth":    map[string]any{"type": "integer", "description": "Limit traversal to N levels below root (0 = unlimited)."},
					"budget":       map[string]any{"type": "integer", "description": "Cap output to ~N tokens, priority-selecting files."},
					"model":        map[string]any{"type": "string", "description": "Annotate fit for a named model (e.g. gpt-4o, claude-3.5-sonnet). Shows whether the bundle fits."},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "repo_map",
			"description": "Return a token-aware tree outline of a repository: each file/directory annotated with a token estimate and byte size.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string"},
					"include":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"exclude":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"max_size":  map[string]any{"type": "integer"},
					"max_depth": map[string]any{"type": "integer", "description": "Limit traversal to N levels below root (0 = unlimited)."},
					"sort":      map[string]any{"type": "string", "enum": []string{"name", "tokens", "bytes"}, "default": "name", "description": "Sort children by: name (default), tokens (largest first), bytes (largest first)."},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "count_tokens",
			"description": "Estimate the total token count of a repository and report whether it fits within each supported model's context window. Use format:json for machine-readable output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string"},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "text", "description": "Output format. json returns the structured envelope."},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "list_models",
			"description": "List the models ctxpack knows about, with each model's context window and its effective limit after the reply reserve. Call this before count_tokens to learn which model names exist.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "diff_repo",
			"description": "Pack only the files changed against a git ref into a context bundle. Deleted files are listed but their content is omitted. Requires a git repository.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Absolute or relative path to the repository root."},
					"ref":    map[string]any{"type": "string", "default": "WORKTREE", "description": "Base git ref (e.g. HEAD~1, main, v1.0.0). Default is WORKTREE for uncommitted changes."},
					"format": map[string]any{"type": "string", "enum": []string{"xml", "markdown", "json", "text"}, "default": "xml"},
					"budget": map[string]any{"type": "integer", "description": "Cap output to ~N tokens, priority-selecting files."},
				},
				"required": []string{"path"},
			},
		},
	}
}

func (s *server) handleToolCall(id any, params any) {
	pmap, _ := params.(map[string]any)
	name, _ := pmap["name"].(string)
	if name == "" {
		// Without a name we would otherwise report 'Unknown tool: ' with an
		// empty name, which tells the agent nothing about what it sent wrong.
		s.writeError(id, -32602, "tools/call is missing arguments.name")
		return
	}
	args, _ := pmap["arguments"].(map[string]any)

	var text string
	var callErr string
	switch name {
	case "pack_repo":
		text, callErr = callPackRepo(args)
	case "repo_map":
		text, callErr = callRepoMap(args)
	case "count_tokens":
		text, callErr = callCountTokens(args)
	case "list_models":
		text, callErr = callListModels()
	case "diff_repo":
		text, callErr = callDiffRepo(args)
	default:
		s.writeError(id, -32602, "Unknown tool: "+name)
		return
	}

	result := map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	}
	if callErr != "" {
		result["isError"] = true
		result["content"] = []map[string]any{
			{"type": "text", "text": callErr},
		}
	}
	s.writeResult(id, result)
}

func callPackRepo(args map[string]any) (string, string) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", "missing required argument: path"
	}
	outFmt, err := parseFormat(getString(args, "format", "xml"))
	if err != nil {
		return "", err.Error()
	}
	bundle, err := packer.Pack(path, packer.Options{
		Walker: walker.Options{
			Include:          toStrSlice(args["include"]),
			Exclude:          toStrSlice(args["exclude"]),
			MaxFileSize:      toInt64(args["max_size"]),
			MaxDepth:         toInt(args["max_depth"]),
			RespectGitignore: !toBool(args["no_gitignore"]),
			IncludeHidden:    toBool(args["hidden"]),
			ReadContent:      true,
		},
		Budget: toInt(args["budget"]),
	})
	if err != nil {
		return "", "pack error: " + err.Error()
	}
	out := format.Render(bundle, outFmt)
	if model := getString(args, "model", ""); model != "" {
		out = annotateFit(bundle.TotalTokens, model) + "\n" + out
	}
	return out, ""
}

func callRepoMap(args map[string]any) (string, string) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", "missing required argument: path"
	}
	root, tokens, bytes, err := repomap.Build(path, repomap.Options{
		Walker: walker.Options{
			Include:          toStrSlice(args["include"]),
			Exclude:          toStrSlice(args["exclude"]),
			MaxFileSize:      toInt64(args["max_size"]),
			MaxDepth:         toInt(args["max_depth"]),
			RespectGitignore: true,
			ReadContent:      false,
		},
		SortBy: toStr(args["sort"]),
	})
	if err != nil {
		return "", "map error: " + err.Error()
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Repository: %s\nFiles: ~%d tokens, %d bytes\n\n", root.Name, tokens, bytes)
	sb.WriteString(repomap.Render(root))
	return sb.String(), ""
}

func callCountTokens(args map[string]any) (string, string) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", "missing required argument: path"
	}
	outFmt := getString(args, "format", "text")
	_, tokens, bytes, err := repomap.Build(path, repomap.Options{
		Walker: walker.Options{
			RespectGitignore: true,
			ReadContent:      false,
		},
	})
	if err != nil {
		return "", "map error: " + err.Error()
	}
	if outFmt == "json" {
		return countTokensJSON(path, tokens, bytes), ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Path: %s\nTokens: ~%d\nBytes: %d\n\nPer-model fit:\n", path, tokens, bytes)
	for _, m := range counter.Models() {
		fit := counter.FitsModel(counter.Estimate{Tokens: tokens}, m, counter.ReplyReserve)
		mark := "fits"
		if !fit.Fits {
			mark = "OVERFLOW"
		}
		fmt.Fprintf(&sb, "  %s — %d/%d (%.0f%%) %s\n", m.Name, fit.Used, fit.Limit, fit.PctUsed, mark)
	}
	return sb.String(), ""
}

// countTokensJSON returns the JSON envelope for count_tokens.
func countTokensJSON(path string, tokens, bytes int) string {
	fits := make([]map[string]any, 0, len(counter.Models()))
	for _, m := range counter.Models() {
		f := counter.FitsModel(counter.Estimate{Tokens: tokens}, m, counter.ReplyReserve)
		fits = append(fits, map[string]any{
			"name":     m.Name,
			"window":   m.ContextWindow,
			"limit":    f.Limit,
			"used":     f.Used,
			"pct_used": math.Round(f.PctUsed*100) / 100,
			"fits":     f.Fits,
			"vendor":   m.Vendor,
		})
	}
	env := map[string]any{
		"path":           path,
		"total_tokens":   tokens,
		"total_bytes":    bytes,
		"reserve_tokens": counter.ReplyReserve,
		"fits":           fits,
	}
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf("error marshaling JSON: %v", err)
	}
	return string(b)
}

// callDiffRepo packs only the files changed against a git ref.
func callDiffRepo(args map[string]any) (string, string) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", "missing required argument: path"
	}
	ref := getString(args, "ref", "WORKTREE")
	outFmt, err := parseFormat(getString(args, "format", "xml"))
	if err != nil {
		return "", err.Error()
	}
	changed, err := gitutil.ChangedFiles(path, ref)
	if err != nil {
		if err == gitutil.ErrNotARepo {
			return "", "not a git repository; 'diff_repo' requires git"
		}
		return "", "git error: " + err.Error()
	}
	if len(changed) == 0 {
		return "", "no changed files vs " + ref
	}
	bundle, err := packer.Pack(path, packer.Options{
		Walker: walker.Options{
			RespectGitignore: true,
			ReadContent:      true,
		},
		Budget: toInt(args["budget"]),
		Files:  changed,
	})
	if err != nil {
		return "", "pack error: " + err.Error()
	}
	return format.Render(bundle, outFmt), ""
}

// callListModels reports the model table. It deliberately takes no arguments:
// there is nothing to filter on, and an argument-taking tool that ignored its
// arguments would hide typos, as ctxpack models did before it took a FlagSet.
func callListModels() (string, string) {
	models := counter.Models()
	var sb strings.Builder
	fmt.Fprintf(&sb, "Models: %d (limit = context_window - %d reply tokens)\n", len(models), counter.ReplyReserve)
	for _, m := range models {
		fmt.Fprintf(&sb, "  %-22s %8d window  %8d limit  (%s)\n",
			m.Name, m.ContextWindow, m.ContextWindow-counter.ReplyReserve, m.Vendor)
	}
	return sb.String(), ""
}

// --- JSON-RPC output helpers ---

func (s *server) writeResult(id any, result any) {
	if id == nil {
		return // notification: no response
	}
	s.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *server) writeError(id any, code int, message string) {
	// Unlike results, errors are always reported: JSON-RPC requires a parse
	// error to come back as {"id": null, "error": {...}} even when the message
	// could not be decoded far enough to recover an id.
	s.write(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": message},
	})
}

func (s *server) write(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	b = append(b, '\n')
	_, _ = s.w.Write(b)
}

// --- arg coercion helpers ---

func toStrSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toStr(v any) string {
	s, _ := v.(string)
	return s
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	}
	return 0
}

func toInt(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	if i, ok := v.(int); ok {
		return i
	}
	return 0
}

func toBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func getString(m map[string]any, key, def string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

func parseFormat(s string) (format.Format, error) {
	switch strings.ToLower(s) {
	case "xml":
		return format.XML, nil
	case "md", "markdown":
		return format.Markdown, nil
	case "json":
		return format.JSON, nil
	case "text", "txt", "plain", "raw":
		return format.Text, nil
	}
	return format.XML, fmt.Errorf("unknown format %q (want xml, markdown, json or text)", s)
}

// annotateFit produces an HTML comment line describing whether tokens fit within
// a named model's context window after the reply reserve.
func annotateFit(tokens int, model string) string {
	m, ok := counter.LookupModel(model)
	if !ok {
		return fmt.Sprintf("<!-- unknown model %q; run `ctxpack models` for known models -->", model)
	}
	fit := counter.FitsModel(counter.Estimate{Tokens: tokens}, m, counter.ReplyReserve)
	mark := "FITS"
	if !fit.Fits {
		mark = "OVERFLOW"
	}
	return fmt.Sprintf("<!-- fit: %s model=%s used=%s/%s (%.0f%%) %s -->",
		mark, m.Name, humanTokens(fit.Used), humanTokens(fit.Limit), fit.PctUsed, mark)
}

func humanTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
}

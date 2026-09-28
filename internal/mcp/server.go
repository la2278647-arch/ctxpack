// Package mcp implements a minimal Model Context Protocol server over stdio.
//
// It speaks JSON-RPC 2.0 with newline-delimited messages (the MCP stdio
// transport) and exposes seven tools that any MCP-capable agent (Claude Desktop,
// Cursor, Codex, ...) can call:
//
//   - pack_repo    -> pack a repository into one context document
//   - repo_map     -> token-aware tree outline
//   - count_tokens -> total tokens plus per-model fit
//   - list_models  -> the model table, with each model's effective limit
//   - diff_repo    -> pack only the files changed against a git ref
//   - doctor       -> the environment this server runs in
//   - version      -> the version this server was built from
//
// Argument names, types, defaults and per-argument descriptions are declared
// once, in tools(), and documented nowhere else. This file used to repeat the
// argument lists by hand and drifted from the published schema; the arguments
// below were once spelled out in full, and thirteen advertised properties were
// left without a description at all, so a client asking what no_gitignore does
// showed an empty hint for an argument the schema said existed.
// TestToolSchemasDocumentEveryProperty fails if a property goes undocumented
// again, and TestEveryAdvertisedPropertyIsAccepted fails if tools() advertises
// an argument a handler does not accept.
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
	"sort"
	"strings"

	"github.com/la2278647-arch/ctxpack/internal/counter"
	"github.com/la2278647-arch/ctxpack/internal/doctor"
	"github.com/la2278647-arch/ctxpack/internal/format"
	"github.com/la2278647-arch/ctxpack/internal/gitutil"
	"github.com/la2278647-arch/ctxpack/internal/packer"
	"github.com/la2278647-arch/ctxpack/internal/repomap"
	"github.com/la2278647-arch/ctxpack/internal/version"
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
					"format":       map[string]any{"type": "string", "enum": []string{"xml", "markdown", "json", "text"}, "default": "xml", "description": "Output format. xml is the default and parses; markdown, json and text are also available."},
					"include":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Globs to include (e.g. \"*.go\")."},
					"exclude":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Globs to exclude."},
					"max_size":     map[string]any{"type": "integer", "description": "Read no more than N bytes of a file; larger files are still listed, without content."},
					"no_gitignore": map[string]any{"type": "boolean", "default": false, "description": "Ignore .gitignore files while walking (built-in ignore rules still apply). Defaults to false."},
					"hidden":       map[string]any{"type": "boolean", "default": false, "description": "Include dotfiles and dot-directories. Defaults to false."},
					"max_depth":    map[string]any{"type": "integer", "description": "Limit traversal to N levels below root (0 = unlimited)."},
					"budget":       map[string]any{"type": "integer", "description": "Cap output to ~N tokens, priority-selecting files."},
					"model":        map[string]any{"type": "string", "description": "Annotate fit for a named model (e.g. gpt-4o, claude-3.5-sonnet). Shows whether the bundle fits."},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "repo_map",
			"description": "Return a token-aware tree outline of a repository: each file/directory annotated with a token estimate and byte size. Use format:json for machine-readable output. top switches the text output to a flat list of the N largest files (largest tokens first, or largest bytes with sort:bytes), the same rendering as the CLI's map --top.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string", "description": "Absolute or relative path to the repository root."},
					"format":    map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "text", "description": "Output format. json returns the structured envelope."},
					"include":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Globs to include (e.g. \"*.go\")."},
					"exclude":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Globs to exclude."},
					"max_size":  map[string]any{"type": "integer", "description": "Read no more than N bytes of a file; larger files are still listed, without content."},
					"max_depth": map[string]any{"type": "integer", "description": "Limit traversal to N levels below root (0 = unlimited)."},
					"sort":      map[string]any{"type": "string", "enum": []string{"name", "tokens", "bytes"}, "default": "name", "description": "Sort children by: name (default), tokens (largest first), bytes (largest first). In top mode this selects what the flat list is ranked by."},
					"top":       map[string]any{"type": "integer", "description": "Text output only: show a flat list of the N largest files instead of the tree. Ignored when format is json. Ranked by tokens, or by bytes when sort is bytes."},
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
					"path":         map[string]any{"type": "string", "description": "Absolute or relative path to the repository root."},
					"format":       map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "text", "description": "Output format. json returns the structured envelope."},
					"include":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Globs to include."},
					"exclude":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Globs to exclude."},
					"max_size":     map[string]any{"type": "integer", "description": "Read no more than N bytes of a file."},
					"max_depth":    map[string]any{"type": "integer", "description": "Limit traversal to N levels below root (0 = unlimited)."},
					"no_gitignore": map[string]any{"type": "boolean", "default": false, "description": "Ignore .gitignore files while walking (built-in ignore rules still apply). Defaults to false."},
					"hidden":       map[string]any{"type": "boolean", "default": false, "description": "Include dotfiles and dot-directories. Defaults to false."},
					"model":        map[string]any{"type": "string", "description": "Show fit for one model only (by name)."},
					"top":          map[string]any{"type": "integer", "description": "Show only the N largest models by context window."},
					"sort":         map[string]any{"type": "string", "enum": []string{"name", "pct", "window"}, "default": "name", "description": "Sort fit table by name, pct_used, or window size."},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "list_models",
			"description": "List the models ctxpack knows about, with each model's context window and its effective limit after the reply reserve. Call this before count_tokens to learn which model names exist. vendor, top and sort filter and order the table the same way the CLI's models command does. Use format:json for machine-readable output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "text", "description": "Output format. json returns the structured envelope."},
					"vendor": map[string]any{"type": "string", "description": "Show only models from this vendor (case-insensitive)."},
					"top":    map[string]any{"type": "integer", "description": "Show only the N models with the largest context windows."},
					"sort":   map[string]any{"type": "string", "enum": []string{"name", "window", "vendor"}, "default": "name", "description": "Sort by name (default), window (largest first), or vendor."},
				},
			},
		},
		{
			"name":        "diff_repo",
			"description": "Pack only the files changed against a git ref into a context bundle. Deleted files are named in a Deleted section, without content. Requires a git repository.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":         map[string]any{"type": "string", "description": "Absolute or relative path to the repository root."},
					"ref":          map[string]any{"type": "string", "default": "WORKTREE", "description": "Base git ref (e.g. HEAD~1, main, v1.0.0) or range (HEAD~5..HEAD). Default is WORKTREE for uncommitted changes; a range compares two revisions and excludes working-tree files."},
					"format":       map[string]any{"type": "string", "enum": []string{"xml", "markdown", "json", "text"}, "default": "xml", "description": "Output format of the packed diff. xml is the default and parses; markdown, json and text are also available."},
					"budget":       map[string]any{"type": "integer", "description": "Cap output to ~N tokens, priority-selecting files."},
					"model":        map[string]any{"type": "string", "description": "Annotate fit for a named model."},
					"include":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Globs to include."},
					"exclude":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Globs to exclude."},
					"max_size":     map[string]any{"type": "integer", "description": "Read no more than N bytes of a file."},
					"no_gitignore": map[string]any{"type": "boolean", "default": false, "description": "Ignore .gitignore files while walking (built-in ignore rules still apply). Defaults to false."},
					"hidden":       map[string]any{"type": "boolean", "default": false, "description": "Include dotfiles and dot-directories. Defaults to false."},
					"max_depth":    map[string]any{"type": "integer", "description": "Limit traversal to N levels below root (0 = unlimited)."},
					"list":         map[string]any{"type": "boolean", "default": false, "description": "List the paths the pack will name, one per line (no packing, no content read). Deletions are named too; the same walk filters as the pack apply."},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "doctor",
			"description": "Report the environment this ctxpack server runs in: version, Go version, platform, git availability and version, and the size of the model registry broken down by vendor. Call this when another tool fails with an environment error such as 'git error' to confirm whether git is installed and which version the server sees. Use format:json for machine-readable output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "text", "description": "Output format. json returns the structured report."},
					"top":    map[string]any{"type": "integer", "description": "In text output only, show just the N vendors with the most models. JSON always reports all of them."},
				},
			},
		},
		{
			"name":        "version",
			"description": "Report the version, build commit and build date of this ctxpack server, and the platform it runs on. Weighs less than doctor, which reports the whole environment: use version when all you need is whether the server is current. Use format:json for machine-readable output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "text", "description": "Output format. json returns the structured envelope."},
				},
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
		text, callErr = callListModels(args)
	case "diff_repo":
		text, callErr = callDiffRepo(args)
	case "doctor":
		text, callErr = callDoctor(args)
	case "version":
		text, callErr = callVersion(args)
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
		// JSON stays parseable: the annotation becomes an envelope field.
		// Prepending the HTML comment the text formats use would make every
		// annotated json call unparsable. See annotateFitJSON.
		if outFmt == format.JSON {
			out = annotateFitJSON(out, bundle.TotalTokens, model)
		} else {
			out = annotateFit(bundle.TotalTokens, model) + "\n" + out
		}
	}
	return out, ""
}

func callRepoMap(args map[string]any) (string, string) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", "missing required argument: path"
	}
	outFmt := getString(args, "format", "text")
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
	if outFmt == "json" {
		return repoMapJSON(root, tokens, bytes), ""
	}
	var sb strings.Builder
	if top := toInt(args["top"]); top > 0 {
		// The flat list ranks by tokens unless sort:bytes is given. The sortBy
		// value is echoed in the header, so default the empty case to the actual
		// ranking key rather than the CLI's "name" default, which would make the
		// header lie about what the list is ordered by.
		sortBy := toStr(args["sort"])
		if sortBy == "" {
			sortBy = "tokens"
		}
		renderRepoMapTop(&sb, root, top, sortBy, countRepoFiles(root))
		return sb.String(), ""
	}
	fmt.Fprintf(&sb, "Repository: %s\nFiles: ~%d tokens, %d bytes\n\n", root.Name, tokens, bytes)
	sb.WriteString(repomap.Render(root))
	return sb.String(), ""
}

// renderRepoMapTop writes the flat top-N file table, mirroring the CLI's
// `map --top` output: every file as an absolute-from-root path, ranked by
// tokens (or by bytes when sortBy is "bytes"), truncated to N.
func renderRepoMapTop(sb *strings.Builder, root *repomap.Node, top int, sortBy string, total int) {
	type flatFile struct {
		path   string
		tokens int
		bytes  int
	}
	var files []flatFile
	var walk func(n *repomap.Node, prefix string)
	walk = func(n *repomap.Node, prefix string) {
		p := prefix + n.Name
		if n.IsDir {
			for _, c := range n.Children {
				walk(c, p+"/")
			}
			return
		}
		files = append(files, flatFile{p, n.Tokens, n.Bytes})
	}
	walk(root, "")
	sort.Slice(files, func(i, j int) bool {
		if sortBy == "bytes" {
			if files[i].bytes != files[j].bytes {
				return files[i].bytes > files[j].bytes
			}
			return files[i].tokens > files[j].tokens
		}
		if files[i].tokens != files[j].tokens {
			return files[i].tokens > files[j].tokens
		}
		return files[i].path < files[j].path
	})
	if top < len(files) {
		files = files[:top]
	}
	fmt.Fprintf(sb, "Top %s by %s (of %d total):\n\n",
		format.Plural(len(files), "file"), sortBy, total)
	fmt.Fprintf(sb, "%-45s %10s %10s\n", "PATH", "TOKENS", "BYTES")
	fmt.Fprintf(sb, "%-45s %10s %10s\n",
		strings.Repeat("-", 45), strings.Repeat("-", 10), strings.Repeat("-", 10))
	for _, f := range files {
		fmt.Fprintf(sb, "%-45s %10d %10d\n", f.path, f.tokens, f.bytes)
	}
}

// countRepoFiles returns the number of files (non-directories) in a tree.
func countRepoFiles(n *repomap.Node) int {
	if !n.IsDir {
		return 1
	}
	total := 0
	for _, c := range n.Children {
		total += countRepoFiles(c)
	}
	return total
}

// repoMapJSON returns the JSON envelope for repo_map, the same shape as the
// CLI's `map --json` output.
func repoMapJSON(root *repomap.Node, tokens, bytes int) string {
	env := map[string]any{
		"root":         root.Name,
		"total_tokens": tokens,
		"total_bytes":  bytes,
		"tree":         toMapNode(root),
	}
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf("error marshaling JSON: %v", err)
	}
	return string(b)
}

// toMapNode mirrors repomap.Node for JSON output. Children is always a slice,
// never null: a file and an empty directory both get [] and the is_dir field
// is what distinguishes them.
func toMapNode(n *repomap.Node) map[string]any {
	out := map[string]any{
		"name":     n.Name,
		"is_dir":   n.IsDir,
		"tokens":   n.Tokens,
		"bytes":    n.Bytes,
		"children": []any{},
	}
	if n.Children != nil {
		kids := make([]any, len(n.Children))
		for i, c := range n.Children {
			kids[i] = toMapNode(c)
		}
		out["children"] = kids
	}
	return out
}

func callCountTokens(args map[string]any) (string, string) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", "missing required argument: path"
	}
	outFmt := getString(args, "format", "text")
	modelFilter := getString(args, "model", "")
	topN := toInt(args["top"])
	sortBy := getString(args, "sort", "name")

	_, tokens, bytes, err := repomap.Build(path, repomap.Options{
		Walker: walker.Options{
			Include:          toStrSlice(args["include"]),
			Exclude:          toStrSlice(args["exclude"]),
			MaxFileSize:      toInt64(args["max_size"]),
			MaxDepth:         toInt(args["max_depth"]),
			RespectGitignore: !toBool(args["no_gitignore"]),
			IncludeHidden:    toBool(args["hidden"]),
			// Read contents so the numbers agree with the CLI's tokens
			// command: an estimate-only walk reported a different total for
			// the same tree, and max_size still caps the reading below.
			ReadContent: true,
		},
	})
	if err != nil {
		return "", "map error: " + err.Error()
	}

	models := filterAndSortModels(counter.Models(), modelFilter, topN, sortBy)

	if outFmt == "json" {
		return countTokensJSON(path, tokens, bytes, models), ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Path: %s\nTokens: ~%d\nBytes: %d\n\nPer-model fit:\n", path, tokens, bytes)
	for _, m := range models {
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
func countTokensJSON(path string, tokens, bytes int, models []counter.Model) string {
	fits := make([]map[string]any, 0, len(models))
	for _, m := range models {
		f := counter.FitsModel(counter.Estimate{Tokens: tokens}, m, counter.ReplyReserve)
		fits = append(fits, map[string]any{
			"model":    m.Name,
			"vendor":   m.Vendor,
			"window":   m.ContextWindow,
			"limit":    f.Limit,
			"used":     f.Used,
			"pct_used": math.Round(f.PctUsed*100) / 100,
			"fits":     f.Fits,
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

// filterAndSortModels filters models by name (if filter is non-empty), truncates
// to the N largest by context window (if top > 0), and otherwise sorts by the
// given field. When top is in effect the window order is the result — exactly
// like the CLI's tokens --top, which ranks by window and ignores --sort — so
// the two interfaces cannot disagree about what a top slice means.
func filterAndSortModels(models []counter.Model, filter string, top int, sortBy string) []counter.Model {
	if filter != "" {
		m, ok := counter.LookupModel(filter)
		if !ok {
			return nil
		}
		models = []counter.Model{m}
	}
	if top > 0 && top < len(models) {
		sort.Slice(models, func(i, j int) bool {
			if models[i].ContextWindow != models[j].ContextWindow {
				return models[i].ContextWindow > models[j].ContextWindow
			}
			return models[i].Name < models[j].Name
		})
		return models[:top]
	}
	switch strings.ToLower(sortBy) {
	case "pct":
		sort.Slice(models, func(i, j int) bool {
			pi := float64(models[i].ContextWindow) / float64(counter.ReplyReserve+models[i].ContextWindow)
			pj := float64(models[j].ContextWindow) / float64(counter.ReplyReserve+models[j].ContextWindow)
			if pi != pj {
				return pi > pj
			}
			return models[i].Name < models[j].Name
		})
	case "window":
		sort.Slice(models, func(i, j int) bool {
			if models[i].ContextWindow != models[j].ContextWindow {
				return models[i].ContextWindow > models[j].ContextWindow
			}
			return models[i].Name < models[j].Name
		})
	default:
		sort.Slice(models, func(i, j int) bool {
			return strings.ToLower(models[i].Name) < strings.ToLower(models[j].Name)
		})
	}
	return models
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
	d, err := gitutil.DiffFiles(path, ref)
	if err != nil {
		if err == gitutil.ErrNotARepo {
			return "", "not a git repository; 'diff_repo' requires git"
		}
		return "", "git error: " + err.Error()
	}
	changed := d.Changed
	if len(changed) == 0 && len(d.Deleted) == 0 {
		// An empty diff is a successful result, not an error: the CLI reports
		// it as a note with exit 0. json consumers must get a parseable
		// envelope (mirroring the CLI's fix), everything else gets the note.
		if outFmt == format.JSON {
			return format.Render(&format.Bundle{}, format.JSON), ""
		}
		return "no changed files vs " + ref, ""
	}
	walkerOpts := walker.Options{
		Include:          toStrSlice(args["include"]),
		Exclude:          toStrSlice(args["exclude"]),
		MaxFileSize:      toInt64(args["max_size"]),
		MaxDepth:         toInt(args["max_depth"]),
		RespectGitignore: !toBool(args["no_gitignore"]),
		IncludeHidden:    toBool(args["hidden"]),
	}
	if toBool(args["list"]) {
		// The same filters the pack applies, so list never disagrees with
		// fileCount. ReadContent stays false: no file body is read.
		res, err := walker.Walk(path, walkerOpts)
		if err != nil {
			return "", "walk error: " + err.Error()
		}
		inScope := make(map[string]bool, len(res.Files))
		for _, fe := range res.Files {
			inScope[fe.RelPath] = true
		}
		var out []string
		for _, f := range changed {
			if inScope[f] {
				out = append(out, f)
			}
		}
		// A deleted file no longer exists, so it cannot appear in the walk; the
		// packer passes deletions through unfiltered and list must match.
		out = append(out, d.Deleted...)
		return strings.Join(out, "\n"), ""
	}
	walkerOpts.ReadContent = true
	bundle, err := packer.Pack(path, packer.Options{
		Walker:  walkerOpts,
		Budget:  toInt(args["budget"]),
		Files:   changed,
		Deleted: d.Deleted,
	})
	if err != nil {
		return "", "pack error: " + err.Error()
	}
	out := format.Render(bundle, outFmt)
	if model := getString(args, "model", ""); model != "" {
		// JSON stays parseable: the annotation becomes an envelope field.
		// Prepending the HTML comment the text formats use would make every
		// annotated json call unparsable. See annotateFitJSON.
		if outFmt == format.JSON {
			out = annotateFitJSON(out, bundle.TotalTokens, model)
		} else {
			out = annotateFit(bundle.TotalTokens, model) + "\n" + out
		}
	}
	return out, ""
}

// callListModels reports the model table. vendor, top and sort are executed,
// not ignored, so a typo in one of them produces a visible change: a vendor
// with no models yields an empty table, an unknown sort falls back to name
// order the way the CLI does. That keeps the "an argument-taking tool that
// ignores its arguments hides typos" failure mode from reappearing.
func callListModels(args map[string]any) (string, string) {
	models := filterModelsForList(counter.Models(),
		getString(args, "vendor", ""), toInt(args["top"]), getString(args, "sort", ""))
	if getString(args, "format", "text") == "json" {
		return listModelsJSON(models), ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Models: %d (limit = context_window - %d reply tokens)\n", len(models), counter.ReplyReserve)
	for _, m := range models {
		fmt.Fprintf(&sb, "  %-22s %8d window  %8d limit  (%s)\n",
			m.Name, m.ContextWindow, m.ContextWindow-counter.ReplyReserve, m.Vendor)
	}
	return sb.String(), ""
}

// filterModelsForList applies the same filtering, truncation and ordering as
// the CLI's `models` command: vendor match is case-insensitive, top takes the
// N largest context windows first and then returns (the CLI applies no further
// sort after top), and sort re-orders the unfiltered result by name (default),
// window descending, or vendor.
func filterModelsForList(models []counter.Model, vendor string, top int, sortBy string) []counter.Model {
	if vendor != "" {
		v := strings.ToLower(vendor)
		out := make([]counter.Model, 0, len(models))
		for _, m := range models {
			if strings.ToLower(m.Vendor) == v {
				out = append(out, m)
			}
		}
		models = out
	}
	if top > 0 {
		sort.Slice(models, func(i, j int) bool {
			if models[i].ContextWindow != models[j].ContextWindow {
				return models[i].ContextWindow > models[j].ContextWindow
			}
			return models[i].Name < models[j].Name
		})
		if top < len(models) {
			models = models[:top]
		}
		return models
	}
	switch strings.ToLower(sortBy) {
	case "window":
		sort.Slice(models, func(i, j int) bool {
			if models[i].ContextWindow != models[j].ContextWindow {
				return models[i].ContextWindow > models[j].ContextWindow
			}
			return models[i].Name < models[j].Name
		})
	case "vendor":
		sort.Slice(models, func(i, j int) bool {
			vi, vj := strings.ToLower(models[i].Vendor), strings.ToLower(models[j].Vendor)
			if vi != vj {
				return vi < vj
			}
			if models[i].ContextWindow != models[j].ContextWindow {
				return models[i].ContextWindow > models[j].ContextWindow
			}
			return models[i].Name < models[j].Name
		})
	default:
		sort.Slice(models, func(i, j int) bool {
			ni, nj := strings.ToLower(models[i].Name), strings.ToLower(models[j].Name)
			if ni != nj {
				return ni < nj
			}
			return models[i].ContextWindow > models[j].ContextWindow
		})
	}
	return models
}

// listModelsJSON returns the model table as JSON, the same shape as the CLI's
// `models --json` output.
func listModelsJSON(models []counter.Model) string {
	entries := make([]map[string]any, 0, len(models))
	for _, m := range models {
		entries = append(entries, map[string]any{
			"name":           m.Name,
			"vendor":         m.Vendor,
			"context_window": m.ContextWindow,
			"limit":          m.ContextWindow - counter.ReplyReserve,
		})
	}
	b, err := json.Marshal(map[string]any{"models": entries})
	if err != nil {
		return fmt.Sprintf("error marshaling JSON: %v", err)
	}
	return string(b)
}

// callVersion reports the version this server was built from. It exists
// because doctor answers the same question inside a wall of environment
// details; version is one short answer, and its JSON form carries exactly the
// fields the CLI's `version --json` does.
func callVersion(args map[string]any) (string, string) {
	f := strings.ToLower(getString(args, "format", "text"))
	if f != "text" && f != "json" {
		return "", fmt.Sprintf("unknown format %q (want text or json)", f)
	}
	if f == "json" {
		return version.JSON(), ""
	}
	return version.Info(), ""
}

// callDoctor reports the environment this server runs in. Unlike the packing
// tools it takes no path: an agent that cannot pack a repository may well be
// unable to name a valid one, so the diagnostic has to work with no argument
// at all. That is the point of the tool — when something else fails with an
// environment error, there must still be one call that can answer.
func callDoctor(args map[string]any) (string, string) {
	rawFormat := getString(args, "format", "text")
	if f := strings.ToLower(rawFormat); f != "text" && f != "json" {
		// doctor offers only text and json. parseFormat would also accept xml
		// and markdown here, which would advertise formats doctor cannot render,
		// so the check is local and names the two it actually supports.
		return "", fmt.Sprintf("unknown format %q (want text or json)", rawFormat)
	}
	report := doctor.Gather()
	if strings.ToLower(rawFormat) == "json" {
		b, err := json.Marshal(report)
		if err != nil {
			return "", "error marshaling JSON: " + err.Error()
		}
		return string(b), ""
	}
	var sb strings.Builder
	report.Text(&sb, toInt(args["top"]))
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

// fitNote computes one model fit and hands back both renderings a caller
// needs: data is the machine-readable form for JSON output and note is the HTML
// comment the text formats use. Deriving both from one place keeps them
// agreeing, the way the CLI's --model note and count_tokens' fit table were
// written as separate copies before this existed.
func fitNote(tokens int, model string) (map[string]any, string) {
	m, ok := counter.LookupModel(model)
	if !ok {
		return map[string]any{"model": model, "unknown": true},
			fmt.Sprintf("<!-- unknown model %q; run `ctxpack models` for known models -->", model)
	}
	fit := counter.FitsModel(counter.Estimate{Tokens: tokens}, m, counter.ReplyReserve)
	mark := "FITS"
	if !fit.Fits {
		mark = "OVERFLOW"
	}
	// The keys match countTokensJSON's per-model fit objects, so the two JSON
	// envelopes speak one vocabulary.
	data := map[string]any{
		"name":     m.Name,
		"vendor":   m.Vendor,
		"window":   m.ContextWindow,
		"limit":    fit.Limit,
		"used":     fit.Used,
		"pct_used": math.Round(fit.PctUsed*100) / 100,
		"fits":     fit.Fits,
		"reserve":  counter.ReplyReserve,
	}
	return data, fmt.Sprintf("<!-- fit: %s model=%s used=%s/%s (%.0f%%) %s -->",
		mark, m.Name, humanTokens(fit.Used), humanTokens(fit.Limit), fit.PctUsed, mark)
}

// annotateFit produces an HTML comment line describing whether tokens fit within
// a named model's context window after the reply reserve.
func annotateFit(tokens int, model string) string {
	_, note := fitNote(tokens, model)
	return note
}

// annotateFitJSON carries the same annotation inside a JSON envelope instead of
// prepending it. The HTML comment a text format accepts would leave the JSON
// unparsable, and format:"json" exists precisely so a client can JSON.parse the
// tool result — before this, every annotated json call threw. The fit object
// still tells the caller whether the pack fits, so nothing is lost.
func annotateFitJSON(out string, tokens int, model string) string {
	data, _ := fitNote(tokens, model)
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		// format.Render never returns invalid JSON for format.JSON; keep the
		// envelope rather than drop the whole result.
		return out
	}
	env["fit"] = data
	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return out
	}
	return string(b)
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

package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveLines feeds the listed JSON-RPC lines through one Serve call and
// returns the parsed responses.
func serveLines(t *testing.T, lines ...string) []map[string]any {
	t.Helper()
	in := strings.Join(lines, "\n") + "\n"
	var out bytes.Buffer
	if err := Serve(strings.NewReader(in), &out, "test"); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var msgs []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("response is not valid JSON: %v: %s", err, line)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func respByID(msgs []map[string]any, id int) map[string]any {
	for _, m := range msgs {
		if f, ok := m["id"].(float64); ok && int(f) == id {
			return m
		}
	}
	return nil
}

func errOf(m map[string]any) (int, string) {
	e, _ := m["error"].(map[string]any)
	if e == nil {
		return 0, ""
	}
	code, _ := e["code"].(float64)
	msg, _ := e["message"].(string)
	return int(code), msg
}

// --- tools/list schema ---

func TestToolsSchema(t *testing.T) {
	want := map[string]bool{
		"pack_repo":    true,
		"repo_map":     true,
		"count_tokens": true,
	}
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	msg := respByID(msgs, 1)
	if msg == nil {
		t.Fatalf("no tools/list response: %v", msgs)
	}
	tools, ok := msg["result"].(map[string]any)["tools"].([]any)
	if !ok {
		t.Fatalf("result.tools missing: %v", msg["result"])
	}
	if len(tools) != len(want) {
		t.Fatalf("tools: got %d, want %d", len(tools), len(want))
	}
	seen := map[string]bool{}
	for _, tv := range tools {
		tool, ok := tv.(map[string]any)
		if !ok {
			t.Fatalf("tool is not an object: %v", tv)
		}
		name, _ := tool["name"].(string)
		if name == "" || !want[name] {
			t.Errorf("unexpected or unnamed tool: %v", tool["name"])
			continue
		}
		if seen[name] {
			t.Errorf("tool %s reported twice", name)
		}
		seen[name] = true
		if _, ok := tool["description"].(string); !ok {
			t.Errorf("%s: missing description", name)
		}
		schema, ok := tool["inputSchema"].(map[string]any)
		if !ok {
			t.Errorf("%s: missing inputSchema", name)
			continue
		}
		if schema["type"] != "object" {
			t.Errorf("%s: inputSchema.type = %v, want object", name, schema["type"])
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Errorf("%s: inputSchema.properties missing", name)
			continue
		}
		if _, ok := props["path"].(map[string]any); !ok {
			t.Errorf("%s: path property missing", name)
		}
		// The schema is the one thing an MCP client actually reads, so
		// required belongs on it - not on the tool object.
		req, ok := schema["required"].([]any)
		if !ok || len(req) == 0 || fmt.Sprint(req[0]) != "path" {
			t.Errorf("%s: inputSchema.required must be [\"path\"]: %v", name, schema["required"])
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("tool %s absent from tools/list", name)
		}
	}
}

// --- tools/call argument validation ---

func TestToolCallUnknownTool(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	msg := respByID(msgs, 1)
	if msg == nil {
		t.Fatal("no response")
	}
	code, text := errOf(msg)
	if code != -32602 || !strings.Contains(text, "nope") {
		t.Errorf("unknown tool: code=%d text=%q", code, text)
	}
}

func TestToolCallMissingName(t *testing.T) {
	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"arguments":{"path":"/tmp"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{}}`)
	for _, id := range []int{1, 2} {
		msg := respByID(msgs, id)
		if msg == nil {
			t.Fatalf("no response for id %d", id)
			continue
		}
		code, text := errOf(msg)
		if code != -32602 || !strings.Contains(text, "arguments.name") {
			t.Errorf("id %d: code=%d text=%q", id, code, text)
		}
	}
}

func TestToolCallNilAndOddParams(t *testing.T) {
	// None of these may panic: they all reach the argument-extraction line.
	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":null}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":["not","an","object"]}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{}}`)
	if len(msgs) != 4 {
		t.Fatalf("responses: got %d, want 4", len(msgs))
	}
	for _, id := range []int{1, 2, 3, 4} {
		if msg := respByID(msgs, id); msg == nil {
			t.Errorf("id %d: no response", id)
		}
	}
}

// --- pack_repo ---

func TestToolCallPackRepo(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	msgs := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"pack_repo","arguments":{"path":%q,"format":"json","include":["*.go"]}}}`, dir))
	text := toolText(t, msgs, "1")
	if text == "" {
		t.Fatalf("no pack_repo output: %v", msgs)
	}
	if !strings.Contains(text, "a.go") || strings.Contains(text, "b.md") {
		t.Errorf("include filter not honoured:\n%s", text)
	}
	var b map[string]any
	if err := json.Unmarshal([]byte(text), &b); err != nil {
		t.Errorf("format=json did not produce JSON: %v\n%s", err, text)
	}
}

func TestToolCallPackRepoBadPath(t *testing.T) {
	msgs := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"pack_repo","arguments":{"path":%q}}}`,
		filepath.Join(t.TempDir(), "no-such-dir")))
	msg := respByID(msgs, 1)
	if msg == nil {
		t.Fatal("no response")
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected an in-tool error, got %v", msg)
	}
	if isErr, _ := res["isError"].(bool); !isErr {
		t.Errorf("bad path must set isError: %v", msg)
	}
	// isError is the only contract here. The wording is an OS error string
	// and differs between Windows and POSIX, so do not assert on it.
	if text := toolText(t, msgs, "1"); !strings.Contains(text, "pack error:") {
		t.Errorf("error text should name the failure: %q", text)
	}
}

func TestToolCallArgsCoercion(t *testing.T) {
	// JSON numbers arrive as float64; the server must coerce them.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("x", 5000)), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"pack_repo","arguments":`+
			`{"path":%q,"max_size":100,"budget":10,"hidden":true,"no_gitignore":false}}}`, dir))
	text := toolText(t, msgs, "1")
	if text == "" {
		t.Fatalf("no output: %v", msgs)
	}
}

// --- repo_map: the ReadContent:false path ---

func TestToolCallRepoMap(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A 20 KiB file: its token estimate must track its size, not its name.
	if err := os.WriteFile(filepath.Join(dir, "pkg", "big.go"),
		[]byte(strings.Repeat("abcdefghij", 2048)), 0o644); err != nil {
		t.Fatal(err)
	}

	msgs := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q}}}`, dir))
	text := toolText(t, msgs, "1")
	if !strings.Contains(text, "Repository:") || !strings.Contains(text, "big.go") {
		t.Errorf("repo_map output: %q", text)
	}
	// Every annotation in the tree is "<digits>t". Extract the largest.
	maxTok := 0
	for _, m := range strings.Split(text, "[") {
		if !strings.Contains(m, "t,") {
			continue
		}
		n := 0
		for _, r := range m {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		if n > maxTok {
			maxTok = n
		}
	}
	// 20 KiB must not be estimated from a 12-character filename.
	if maxTok < 1000 {
		t.Errorf("largest token estimate is %d; a 20 KiB file must be estimated by size", maxTok)
	}
}

// --- protocol edges ---

func TestUnknownNotificationGetsNoResponse(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","method":"no/such/notification"}`)
	if len(msgs) != 0 {
		t.Errorf("a notification must not be answered, got %v", msgs)
	}
}

func TestBlankLinesAreIgnored(t *testing.T) {
	msgs := serveLines(t, "", `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`, "", "")
	if len(msgs) != 1 {
		t.Fatalf("responses: got %d, want 1", len(msgs))
	}
}

func TestPingResultIsEmptyObject(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":7,"method":"ping","params":{}}`)
	msg := respByID(msgs, 7)
	if msg == nil {
		t.Fatal("no response")
	}
	if r, ok := msg["result"].(map[string]any); !ok || len(r) != 0 {
		t.Errorf("ping result = %v", msg["result"])
	}
}

func TestInitializePayload(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	msg := respByID(msgs, 1)
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if res["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v", res["protocolVersion"])
	}
	info, _ := res["serverInfo"].(map[string]any)
	if info["name"] != "ctxpack" || info["version"] != "test" {
		t.Errorf("serverInfo = %v", res["serverInfo"])
	}
	if _, ok := res["capabilities"].(map[string]any); !ok {
		t.Errorf("capabilities missing: %v", res["capabilities"])
	}
}

// --- helpers ---

func TestCoercionHelpers(t *testing.T) {
	if got := toStrSlice([]any{"a", 1, "b"}); strings.Join(got, ",") != "a,b" {
		t.Errorf("toStrSlice dropped non-strings wrongly: %v", got)
	}
	if got := toStrSlice("not an array"); got != nil {
		t.Errorf("toStrSlice(non-array) = %v", got)
	}
	for _, tc := range []struct {
		in   any
		want int64
	}{
		{float64(42), 42}, {int(7), 7}, {int64(99), 99}, {"nope", 0}, {nil, 0},
	} {
		if got := toInt64(tc.in); got != tc.want {
			t.Errorf("toInt64(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
	if got := toInt(float64(12.9)); got != 12 {
		t.Errorf("toInt(float64) = %d", got)
	}
	if got := toInt("nope"); got != 0 {
		t.Errorf("toInt(string) = %d", got)
	}
	if !toBool(true) || toBool("yes") || toBool(nil) {
		t.Error("toBool is wrong")
	}
	if got := getString(map[string]any{"k": "v"}, "k", "d"); got != "v" {
		t.Errorf("getString = %q", got)
	}
	if got := getString(map[string]any{"k": ""}, "k", "d"); got != "d" {
		t.Errorf("empty string must fall back to the default: %q", got)
	}
	if got := getString(nil, "k", "d"); got != "d" {
		t.Errorf("nil map: %q", got)
	}
}

func TestParseFormat(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"xml", "xml"}, {"XML", "xml"}, {"markdown", "markdown"}, {"MD", "markdown"},
		{"json", "json"}, {"text", "text"}, {"txt", "text"}, {"plain", "text"},
		{"", "xml"}, {"yaml", "xml"},
	} {
		if got := parseFormat(tc.in); string(got) != tc.want {
			t.Errorf("parseFormat(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

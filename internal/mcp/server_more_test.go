package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/la2278647-arch/ctxpack/internal/counter"
)

// Every tool must name a required argument when it is absent.
func TestToolCallsReportAMissingPath(t *testing.T) {
	for _, tool := range []string{"pack_repo", "repo_map", "count_tokens"} {
		msgs := serveLines(t, fmt.Sprintf(
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+
				`{"name":"%s","arguments":{}}}`, tool))
		msg := respByID(msgs, 1)
		if msg == nil {
			t.Fatalf("%s: no response", tool)
			continue
		}
		res, ok := msg["result"].(map[string]any)
		if !ok {
			t.Fatalf("%s: no result object: %v", tool, msg)
			continue
		}
		if isErr, _ := res["isError"].(bool); !isErr {
			t.Errorf("%s: a missing path must set result.isError: %v", tool, msg)
		}
		text := toolText(t, msgs, "1")
		if !strings.Contains(text, "missing required argument: path") {
			t.Errorf("%s: error text = %q", tool, text)
		}
	}
}

// repo_map and count_tokens force RespectGitignore: true, so a root
// .gitignore the matcher cannot read makes Build fail. One line past the
// 1 MiB scanner cap is enough.
func TestToolCallsReportARepositoryError(t *testing.T) {
	for _, tool := range []string{"repo_map", "count_tokens"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"),
			[]byte(strings.Repeat("x", 1024*1024+1)), 0o644); err != nil {
			t.Fatal(err)
		}
		msgs := serveLines(t, fmt.Sprintf(
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+
				`{"name":"%s","arguments":{"path":%q}}}`, tool, dir))
		msg := respByID(msgs, 1)
		if msg == nil {
			t.Fatalf("%s: no response", tool)
			continue
		}
		res, ok := msg["result"].(map[string]any)
		if !ok {
			t.Fatalf("%s: no result object: %v", tool, msg)
			continue
		}
		if isErr, _ := res["isError"].(bool); !isErr {
			t.Errorf("%s: an unreadable repository must set result.isError: %v", tool, msg)
		}
		text := toolText(t, msgs, "1")
		if !strings.Contains(text, "error:") {
			t.Errorf("%s: the error text should name the failure: %q", tool, text)
		}
	}
}

// repo_map top switches the text output to a flat largest-files table, the
// same rendering as the CLI's map --top, while format:json keeps the full tree.
func TestRepoMapTopListsLargestFiles(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"small.txt": "hi\n",
		"medium.go": strings.Repeat("package main\nvar x = 1\n", 15),
		"large.go":  "package main\n" + strings.Repeat("// fill the buffer\nvar data = `x`\n", 300),
	}
	for p, body := range files {
		if err := os.WriteFile(filepath.Join(dir, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	msgs := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q,"top":1}}}`,
		filepath.ToSlash(dir)))
	text := toolText(t, msgs, "1")
	if !strings.Contains(text, "Top 1 file by tokens") {
		t.Errorf("top=1 output missing the header:\n%s", text)
	}
	if !strings.Contains(text, "large.go") || strings.Contains(text, "medium.go") {
		t.Errorf("top=1 must name only the largest file:\n%s", text)
	}

	msgs = serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q,"top":2,"sort":"bytes"}}}`,
		filepath.ToSlash(dir)))
	text = toolText(t, msgs, "1")
	if !strings.Contains(text, "by bytes") {
		t.Errorf("sort=bytes not reflected in the top header:\n%s", text)
	}
	if !strings.Contains(text, "large.go") || !strings.Contains(text, "medium.go") {
		t.Errorf("top=2 must name large.go and medium.go:\n%s", text)
	}
	if strings.Contains(text, "small.txt") {
		t.Errorf("top=2 must not name the smallest file:\n%s", text)
	}

	// JSON ignores top: the full tree is returned so the envelope stays the
	// same shape whether or not top was passed.
	msgs = serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q,"top":1,"format":"json"}}}`,
		filepath.ToSlash(dir)))
	text = toolText(t, msgs, "1")
	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("repo_map json with top is not valid JSON: %v\n%s", err, text)
	}
	if _, ok := env["tree"]; !ok {
		t.Errorf("repo_map json with top is missing the full tree envelope:\n%s", text)
	}
}

// TestRepoMapEmptyDirJSON pins that mapping an empty directory in json format
// yields a parseable envelope with a tree node (children []), not an error.
func TestRepoMapEmptyDirJSON(t *testing.T) {
	dir := t.TempDir()
	msgs := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q,"format":"json"}}}`,
		filepath.ToSlash(dir)))
	text := toolText(t, msgs, "1")
	var env struct {
		Tree struct {
			Children []any `json:"children"`
		} `json:"tree"`
	}
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("repo_map empty-dir json is not a parseable envelope: %v\n%s", err, text)
	}
	if len(env.Tree.Children) != 0 {
		t.Errorf("empty dir repo_map tree has %d children, want 0", len(env.Tree.Children))
	}
}

// TestRepoMapMaxSizeUsesEstimate pins repo_map's max_size argument: a file
// over the cap is estimated from its byte size, so the envelope's total drops
// below the full-read count (the mcp mirror of TestMapMaxSizeUsesEstimate).
func TestRepoMapMaxSizeUsesEstimate(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("a", 500)), 0o644)

	full := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q,"format":"json"}}}`,
		filepath.ToSlash(dir)))
	capped := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q,"format":"json","max_size":10}}}`,
		filepath.ToSlash(dir)))
	parse := func(msgs []map[string]any) int {
		var env struct {
			TotalTokens int `json:"total_tokens"`
		}
		if err := json.Unmarshal([]byte(toolText(t, msgs, "1")), &env); err != nil {
			t.Fatalf("repo_map json is not parseable: %v", err)
		}
		return env.TotalTokens
	}
	if got, want := parse(capped), parse(full); got >= want {
		t.Errorf("max_size 10 must cut the total below the full-read %d, got %d", want, got)
	}
}

// TestRepoMapNoGitignoreHiddenFlags pins that repo_map's no_gitignore and
// hidden arguments actually take effect (they used to be ignored: the walker
// hard-coded RespectGitignore and omitted IncludeHidden).
func TestRepoMapNoGitignoreHiddenFlags(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("secret.txt\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("s\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden.go"), []byte("package h\n"), 0o644)

	noGit := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q,"no_gitignore":true}}}`,
		filepath.ToSlash(dir)))
	text := toolText(t, noGit, "1")
	if !strings.Contains(text, "secret.txt") {
		t.Errorf("no_gitignore:true must include the gitignored file:\n%s", text)
	}

	hidden := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q,"hidden":true}}}`,
		filepath.ToSlash(dir)))
	text = toolText(t, hidden, "1")
	if !strings.Contains(text, ".hidden.go") {
		t.Errorf("hidden:true must include the dotfile:\n%s", text)
	}
}

// TestRepoMapIncludeFilter pins repo_map's include argument: only matching
// files appear in the outline (the mcp mirror of the CLI map --include tests).
func TestRepoMapIncludeFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0o644)

	msgs := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q,"include":["*.go"]}}}`,
		filepath.ToSlash(dir)))
	text := toolText(t, msgs, "1")
	if !strings.Contains(text, "a.go") {
		t.Errorf("include *.go must list a.go:\n%s", text)
	}
	if strings.Contains(text, "b.txt") {
		t.Errorf("include *.go must drop b.txt:\n%s", text)
	}
}

// TestRepoMapExcludeFilter pins repo_map's exclude argument: matching files
// stay out of the outline (the mirror of TestRepoMapIncludeFilter).
func TestRepoMapExcludeFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0o644)

	msgs := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q,"exclude":["*.txt"]}}}`,
		filepath.ToSlash(dir)))
	text := toolText(t, msgs, "1")
	if !strings.Contains(text, "a.go") {
		t.Errorf("exclude *.txt must keep a.go:\n%s", text)
	}
	if strings.Contains(text, "b.txt") {
		t.Errorf("exclude *.txt must drop b.txt:\n%s", text)
	}
}

// TestRepoMapMaxDepth pins repo_map's max_depth argument: a two-level file is
// not traversed at max_depth 1, while one-level files are (the mcp mirror of
// the CLI depth boundary tests).
func TestRepoMapMaxDepth(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "deep", "x.txt"), []byte("x\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "a.go"), []byte("package a\n"), 0o644)

	msgs := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q,"max_depth":1}}}`,
		filepath.ToSlash(dir)))
	text := toolText(t, msgs, "1")
	if !strings.Contains(text, "a.go") {
		t.Errorf("max_depth 1 must keep the one-level file:\n%s", text)
	}
	if strings.Contains(text, "x.txt") {
		t.Errorf("max_depth 1 must not reach the two-level file:\n%s", text)
	}
}

// TestRepoMapJSONSortsByBytes pins that repo_map's sort argument orders the
// JSON tree the same way it orders the text outline (and the CLI's map --json
// does): children come largest-first when sorting by bytes.
func TestRepoMapJSONSortsByBytes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("a", 500)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "small.txt"), []byte(strings.Repeat("b", 20)), 0o644); err != nil {
		t.Fatal(err)
	}

	msgs := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo_map","arguments":{"path":%q,"format":"json","sort":"bytes"}}}`,
		filepath.ToSlash(dir)))
	text := toolText(t, msgs, "1")
	var env struct {
		Tree struct {
			Children []struct {
				Name  string `json:"name"`
				Bytes int    `json:"bytes"`
			} `json:"children"`
		} `json:"tree"`
	}
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("repo_map json is not valid JSON: %v\n%s", err, text)
	}
	if len(env.Tree.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(env.Tree.Children))
	}
	if env.Tree.Children[0].Name != "big.txt" || env.Tree.Children[0].Bytes != 500 {
		t.Errorf("sort=bytes should put the largest file first, got %+v", env.Tree.Children[0])
	}
}

// A repository large enough to exceed the smallest registered window must show
// OVERFLOW, and the same run should still fit the largest model.
func TestCountTokensReportsOverflow(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 60; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("doc%02d.txt", i)),
			[]byte(strings.Repeat("the quick brown fox jumps over the lazy dog. ", 200)),
			0o644); err != nil {
			t.Fatal(err)
		}
	}

	msgs := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+
			`{"name":"count_tokens","arguments":{"path":%q}}}`, dir))
	text := toolText(t, msgs, "1")
	if text == "" {
		t.Fatalf("no count_tokens output: %v", msgs)
	}
	if !strings.Contains(text, "OVERFLOW") {
		t.Errorf("a large repository should overflow the smallest window:\n%s", text)
	}
	if !strings.Contains(text, " fits") {
		t.Errorf("the same repository should still fit the largest model:\n%s", text)
	}
}

// "id": null is a notification: the server must not answer it.
func TestExplicitNullIDIsANotification(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":null,"method":"ping","params":{}}`)
	if len(msgs) != 0 {
		t.Errorf("a null id is a notification and must not be answered, got %v", msgs)
	}
}

// json.Marshal fails on NaN. The write must drop the message whole rather than
// emit a half-line that would corrupt the next response.
func TestWriteSwallowsAMarshalFailure(t *testing.T) {
	var out bytes.Buffer
	s := &server{w: &out, version: "test"}
	s.write(map[string]any{"n": math.NaN()})
	if out.Len() != 0 {
		t.Errorf("a marshal failure must not write anything, got %q", out.String())
	}
	// The writer is still usable afterwards.
	s.write(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{}})
	if !strings.HasSuffix(out.String(), "\n") || !strings.Contains(out.String(), `"id":1`) {
		t.Errorf("the writer was left broken: %q", out.String())
	}
}

// JSON numbers arrive as float64, but an argument map built in Go (a test, or
// an embedded use) holds an int. Both must coerce.
func TestToIntAcceptsAnInt(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want int
	}{
		{int(42), 42},
		{int(0), 0},
		{float64(12.9), 12},
		{float64(0), 0},
		{"not a number", 0},
		{nil, 0},
		{true, 0},
	} {
		if got := toInt(tc.in); got != tc.want {
			t.Errorf("toInt(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// The tool schema must not promise to skip large files: the walker lists them,
// it just does not read them.
func TestPackRepoSchemaDescribesMaxSizeHonestly(t *testing.T) {
	for _, tool := range tools() {
		if tool["name"] != "pack_repo" {
			continue
		}
		schema := tool["inputSchema"].(map[string]any)
		props := schema["properties"].(map[string]any)
		maxSize := props["max_size"].(map[string]any)
		desc, ok := maxSize["description"].(string)
		if !ok {
			t.Fatalf("max_size has no description: %v", maxSize)
		}
		if strings.Contains(desc, "Skip") {
			t.Errorf("max_size still claims to skip files: %q", desc)
		}
		if !strings.Contains(desc, "without content") {
			t.Errorf("max_size should say the content is what is dropped: %q", desc)
		}
	}
}

// --- list_models ---

// An MCP client is an LLM choosing a model name by hand. Before list_models it
// could not discover which names existed at all.
func TestListModelsToolListsEveryModel(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_models","arguments":{}}}`)
	msg := respByID(msgs, 1)
	if msg == nil {
		t.Fatal("no response")
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result object: %v", msg)
	}
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("list_models reported an error: %v", res)
	}
	text := toolText(t, msgs, "1")
	if text == "" {
		t.Fatalf("list_models returned no text: %v", res)
	}

	models := counter.Models()
	if want := fmt.Sprintf("Models: %d", len(models)); !strings.Contains(text, want) {
		t.Errorf("text does not state the model count %q:\n%s", want, text)
	}
	if want := fmt.Sprintf("%d reply tokens", counter.ReplyReserve); !strings.Contains(text, want) {
		t.Errorf("text does not state the reply reserve %q:\n%s", want, text)
	}
	for _, m := range models {
		if !strings.Contains(text, m.Name) {
			t.Errorf("list_models is missing %q", m.Name)
		}
	}
}

func TestListModelsJSON(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_models","arguments":{"format":"json"}}}`)
	msg := respByID(msgs, 1)
	if msg == nil {
		t.Fatal("no response")
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result object: %v", msg)
	}
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("list_models reported an error: %v", res)
	}
	text := toolText(t, msgs, "1")
	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("list_models JSON is not valid JSON: %v\n%s", err, text)
	}
	entries, ok := env["models"].([]any)
	if !ok {
		t.Fatalf("models missing from JSON: %v", env)
	}
	models := counter.Models()
	if len(entries) != len(models) {
		t.Errorf("models entries = %d, want %d", len(entries), len(models))
	}
	first, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("model entry is not an object: %v", entries[0])
	}
	for _, field := range []string{"name", "vendor", "context_window", "limit"} {
		if _, ok := first[field]; !ok {
			t.Errorf("model entry missing %q: %v", field, first)
		}
	}
	// The limit in JSON must equal context_window - ReplyReserve.
	if m, ok := counter.LookupModel(first["name"].(string)); ok {
		if int(first["limit"].(float64)) != m.ContextWindow-counter.ReplyReserve {
			t.Errorf("limit mismatch for %s", first["name"])
		}
	}
}

func listModelsJSONEntries(t *testing.T, args string) []map[string]any {
	t.Helper()
	msgs := serveLines(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_models","arguments":{%s}}}`, args))
	text := toolText(t, msgs, "1")
	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("list_models JSON is not valid JSON: %v\n%s", err, text)
	}
	entries, _ := env["models"].([]any)
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.(map[string]any))
	}
	return out
}

func TestListModelsVendorFilter(t *testing.T) {
	entries := listModelsJSONEntries(t, `"format":"json","vendor":"GOOGLE"`)
	if len(entries) == 0 {
		t.Fatal("vendor=GOOGLE returned no models")
	}
	for _, e := range entries {
		if e["vendor"] != "google" {
			t.Errorf("vendor filter leaked %q, want only google", e["vendor"])
		}
	}
	if len(entries) >= len(counter.Models()) {
		t.Errorf("vendor filter returned %d entries, want fewer than the full %d", len(entries), len(counter.Models()))
	}
}

// TestListModelsVendorSort pins that sort re-orders the vendor-filtered set,
// not the whole table: every model is from the vendor, and windows are
// non-increasing when sorting by window.
func TestListModelsVendorSort(t *testing.T) {
	entries := listModelsJSONEntries(t, `"format":"json","vendor":"anthropic","sort":"window"`)
	if len(entries) == 0 {
		t.Fatal("no anthropic models")
	}
	prev := 1 << 62
	for _, e := range entries {
		if e["vendor"] != "anthropic" {
			t.Fatalf("vendor filter leaked %q", e["vendor"])
		}
		win := int(e["context_window"].(float64))
		if win > prev {
			t.Errorf("sort=window is not non-increasing: %d after %d", win, prev)
		}
		prev = win
	}
}

func TestListModelsTopAndSort(t *testing.T) {
	top := listModelsJSONEntries(t, `"format":"json","top":3`)
	if len(top) != 3 {
		t.Fatalf("top=3 returned %d entries, want 3", len(top))
	}
	// top sorts by window descending, so the first entry is the largest window.
	maxWin := 0
	for _, m := range counter.Models() {
		if m.ContextWindow > maxWin {
			maxWin = m.ContextWindow
		}
	}
	if int(top[0]["context_window"].(float64)) != maxWin {
		t.Errorf("top[0] window = %v, want the largest %d", top[0]["context_window"], maxWin)
	}

	sorted := listModelsJSONEntries(t, `"format":"json","sort":"window"`)
	if len(sorted) == 0 {
		t.Fatal("sort=window returned no models")
	}
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1]["context_window"].(float64) < sorted[i]["context_window"].(float64) {
			t.Errorf("sort=window is not descending at %d: %v < %v",
				i, sorted[i-1]["context_window"], sorted[i]["context_window"])
		}
	}

	byVendor := listModelsJSONEntries(t, `"format":"json","sort":"vendor"`)
	prev := ""
	for i, e := range byVendor {
		v := e["vendor"].(string)
		if i > 0 && v < prev {
			t.Errorf("sort=vendor is not ascending at %d: %q before %q", i, prev, v)
		}
		prev = v
	}
}

// The reserve was a literal in count_tokens and a constant in the CLI, so a
// change to one would not have shown in the other. Both tools now read
// counter.ReplyReserve, and this checks that every model's limit really does
// come out the same from both.
func TestListModelsLimitsMatchCountTokensLimits(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs := serveLines(t,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q}}}`, dir),
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_models","arguments":{}}}`,
	)

	fit := toolText(t, msgs, "1")
	table := toolText(t, msgs, "2")
	if fit == "" || table == "" {
		t.Fatalf("empty output: count_tokens=%q list_models=%q", fit, table)
	}

	fromFit := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s+(\S+) — (\d+)/(\d+) \(`).FindAllStringSubmatch(fit, -1) {
		fromFit[m[1]] = m[3]
	}
	fromTable := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s+(\S+)\s+\d+ window\s+(\d+) limit`).FindAllStringSubmatch(table, -1) {
		fromTable[m[1]] = m[2]
	}

	models := counter.Models()
	if len(fromFit) != len(models) || len(fromTable) != len(models) {
		t.Fatalf("parsed %d fit limits and %d table limits, want %d",
			len(fromFit), len(fromTable), len(models))
	}
	for _, m := range models {
		want := fmt.Sprint(m.ContextWindow - counter.ReplyReserve)
		if got := fromFit[m.Name]; got != want {
			t.Errorf("%s: count_tokens limit %q, want %s", m.Name, got, want)
		}
		if got := fromTable[m.Name]; got != want {
			t.Errorf("%s: list_models limit %q, want %s", m.Name, got, want)
		}
		if fromFit[m.Name] != fromTable[m.Name] {
			t.Errorf("%s: count_tokens %q != list_models %q",
				m.Name, fromFit[m.Name], fromTable[m.Name])
		}
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
}

func gitAddAll(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
}

func gitCommit(t *testing.T, dir, msg string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-m", msg)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

func TestToolCallDiffRepo(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`"}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "a.txt") {
		t.Errorf("diff_repo should include a.txt: %s", text)
	}
}

func TestToolCallDiffRepoNotGit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`"}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] != true {
		t.Errorf("expected error for non-git dir, got: %v", result["content"])
	}
}

func TestToolCallDiffRepoNoChanges(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	// No changes after commit.

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`"}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Errorf("no changes is a successful result, not an error: %v", result["content"])
	}
	text := toolText(t, msgs, "2")
	if !strings.Contains(text, "no changed files") {
		t.Errorf("expected a no-changes note, got: %q", text)
	}

	// json format must yield a parseable envelope even for an empty diff.
	msgs = serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","format":"json"}}}`,
	)
	text = toolText(t, msgs, "2")
	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Errorf("empty diff json must be a parseable envelope: %v\n%s", err, text)
	}
}

func TestToolCallDiffRepoModel(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","model":"gpt-4o"}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "fit:") {
		t.Errorf("expected fit annotation:\n%s", text)
	}
	if !strings.Contains(text, "gpt-4o") {
		t.Errorf("expected model name:\n%s", text)
	}
}

func TestToolCallDiffRepoList(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("new file"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","list":true}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "a.txt") {
		t.Errorf("expected a.txt in list:\n%s", text)
	}
	if !strings.Contains(text, "b.txt") {
		t.Errorf("expected b.txt in list:\n%s", text)
	}
	if strings.Contains(text, "tokens") {
		t.Errorf("list mode should not show tokens:\n%s", text)
	}
}

// TestToolCallDiffRepoMaxSize pins diff_repo's max_size argument: a changed
// file over the cap stays in the diff but its body is not read, so the token
// total is tiny and the envelope marks it skipped (the mcp mirror of
// TestDiffMaxSizeOmitsContent).
func TestToolCallDiffRepoMaxSize(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Repeat("a", 50)), 0o644)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Repeat("b", 500)), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","format":"json","max_size":10}}}`,
	)
	text := toolText(t, msgs, "2")
	var env struct {
		Files       []any `json:"files"`
		TotalTokens int   `json:"total_tokens"`
		Skipped     int   `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("diff_repo json is not parseable: %v\n%s", err, text)
	}
	if len(env.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(env.Files))
	}
	if env.TotalTokens >= 100 {
		t.Errorf("max_size 10 must omit the oversized body; total_tokens = %d", env.TotalTokens)
	}
	if env.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", env.Skipped)
	}
}

// TestToolCallDiffRepoMaxDepth pins diff_repo's max_depth argument: a changed
// file two levels deep is out of the list at max_depth 1 (the mcp mirror of
// TestDiffDepthLimitsNested).
func TestToolCallDiffRepoMaxDepth(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "deep", "x.txt"), []byte("x\n"), 0o644)
	gitInit(t, dir)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	os.WriteFile(filepath.Join(dir, "sub", "deep", "x.txt"), []byte("x\ny\n"), 0o644)

	shallow := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":%q,"list":true,"max_depth":1}}}`,
		filepath.ToSlash(dir)))
	if got := toolText(t, shallow, "1"); strings.Contains(got, "x.txt") {
		t.Errorf("max_depth 1 must not list the two-level change, got:\n%s", got)
	}

	deeper := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":%q,"list":true,"max_depth":2}}}`,
		filepath.ToSlash(dir)))
	if got := toolText(t, deeper, "1"); !strings.Contains(got, "x.txt") {
		t.Errorf("max_depth 2 must list the two-level change, got:\n%s", got)
	}
}

// TestToolCallDiffRepoBudget pins diff_repo's budget argument: a changed file
// too big for the budget is omitted (and named in the omitted list) while the
// envelope stays parseable — the mcp mirror of the CLI diff budget tests.
func TestToolCallDiffRepoBudget(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Repeat("a", 50)), 0o644)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Repeat("b", 500)), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","format":"json","budget":1}}}`,
	)
	text := toolText(t, msgs, "2")
	var env struct {
		Files    *[]any `json:"files"`
		Omitted  []any  `json:"omitted"`
		SkipFlag bool   `json:"-"`
	}
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("diff_repo budget json is not parseable: %v\n%s", err, text)
	}
	got := 0
	if env.Files != nil {
		got = len(*env.Files)
	}
	if got != 0 {
		t.Errorf("budget 1 must drop the changed file; files = %d", got)
	}
	if len(env.Omitted) != 1 {
		t.Errorf("omitted = %v, want [f.txt]", env.Omitted)
	}
}

// TestToolCallDiffRepoListIgnoresFormat pins that list wins over format at
// the MCP layer too (matching the CLI): with list:true and format:json, the
// answer is a plain path list, not a JSON envelope.
func TestToolCallDiffRepoListIgnoresFormat(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("new file"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","list":true,"format":"json"}}}`,
	)
	text := toolText(t, msgs, "2")
	if !strings.Contains(text, "b.txt") {
		t.Errorf("list:true must name the new file:\n%s", text)
	}
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		t.Errorf("list:true must win over format:json, got an envelope:\n%s", text)
	}
}

// A range ref compares two revisions as history, so the working tree's
// untracked file must not leak in — it exists in neither side of the range.
// This pins the gitutil range semantics at the MCP layer.
func TestToolCallDiffRepoRangeExcludesWorktree(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)
	gitAddAll(t, dir)
	gitCommit(t, dir, "second")
	// Untracked in no commit, but present in the working tree.
	os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("not committed"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","ref":"HEAD~1..HEAD","list":true}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "a.txt") {
		t.Errorf("range should include a.txt, which differs across the range:\n%s", text)
	}
	if strings.Contains(text, "untracked") {
		t.Errorf("range must not include the working tree's untracked files:\n%s", text)
	}
}

// list must honour include like the pack does: a filter that the caller
// expects to narrow the bundle cannot silently be ignored by the one call that
// pretends not to build it.
func TestToolCallDiffRepoListHonoursInclude(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte("root"), 0o644)
	os.WriteFile(filepath.Join(dir, "pkg", "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "pkg", "b.txt"), []byte("b"), 0o644)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	os.WriteFile(filepath.Join(dir, "pkg", "a.txt"), []byte("a changed"), 0o644)
	os.WriteFile(filepath.Join(dir, "pkg", "b.txt"), []byte("b changed"), 0o644)
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte("root changed"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","list":true,"include":["pkg/*"]}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "pkg/a.txt") || !strings.Contains(text, "pkg/b.txt") {
		t.Errorf("both pkg changes should be listed:\n%s", text)
	}
	if strings.Contains(text, "root.txt") {
		t.Errorf("include pkg/* must exclude root.txt:\n%s", text)
	}
}

// list must also name deletions, which the pack reports in a Deleted section.
func TestToolCallDiffRepoListIncludesDeletions(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	os.WriteFile(filepath.Join(dir, "gone.txt"), []byte("bye"), 0o644)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)
	os.Remove(filepath.Join(dir, "gone.txt"))

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","list":true}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "a.txt") {
		t.Errorf("list should include the modified file:\n%s", text)
	}
	if !strings.Contains(text, "gone.txt") {
		t.Errorf("list should name the deleted file:\n%s", text)
	}
}

// A deletion has no content to pack, but the tool must still say so.
func TestToolCallDiffRepoReportsDeletions(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "gone.txt"), []byte("bye"), 0o644)
	gitInit(t, dir)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)
	os.Remove(filepath.Join(dir, "gone.txt"))

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","format":"markdown"}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "a.txt") {
		t.Errorf("diff_repo should still include a.txt:\n%s", text)
	}
	if !strings.Contains(text, "## Deleted (1 file)") {
		t.Errorf("expected a deleted section:\n%s", text)
	}
	if strings.Contains(text, "## Deleted (1 files)") {
		t.Errorf("deleted section must not pluralise a single file:\n%s", text)
	}
	if !strings.Contains(text, "`gone.txt`") {
		t.Errorf("deleted section missing gone.txt:\n%s", text)
	}
}

func TestToolCallDiffRepoMissingPath(t *testing.T) {
	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] != true {
		t.Errorf("expected error for missing path")
	}
}

func TestToolCallDiffRepoBadFormat(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","format":"invalid"}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] != true {
		t.Errorf("expected error for invalid format")
	}
}

func TestToolCallCountTokensSortByPct(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":"`+filepath.ToSlash(dir)+`","sort":"pct","top":3}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	// Should have exactly 3 model lines.
	lines := strings.Split(text, "\n")
	modelLines := 0
	for _, line := range lines {
		if strings.Contains(line, " — ") {
			modelLines++
		}
	}
	if modelLines != 3 {
		t.Errorf("expected 3 models with top=3, got %d:\n%s", modelLines, text)
	}
}

func TestToolCallCountTokensUnknownModel(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":"`+filepath.ToSlash(dir)+`","model":"unknown-model"}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	// Unknown model returns empty fits (no error).
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if strings.Contains(text, " — ") {
		t.Errorf("should not list any models for unknown model:\n%s", text)
	}
}

func TestToolCallCountTokensJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":"`+filepath.ToSlash(dir)+`","format":"json"}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	// Should be valid JSON with total_tokens, total_bytes, fits.
	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("count_tokens JSON is not valid JSON: %v\n%s", err, text)
	}
	if _, ok := env["total_tokens"]; !ok {
		t.Error("missing total_tokens in JSON")
	}
	if _, ok := env["fits"]; !ok {
		t.Error("missing fits in JSON")
	}
	fits, _ := env["fits"].([]any)
	if len(fits) == 0 {
		t.Error("fits array is empty")
	}
}

// TestToolCallPackRepoMaxSize pins pack_repo's max_size argument: a file over
// the cap is still in the bundle, but its body is not read (small token count
// and a skipped entry in the envelope).
func TestToolCallPackRepoMaxSize(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("a", 500)), 0o644)

	msgs := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"pack_repo","arguments":{"path":%q,"format":"json","max_size":10}}}`,
		filepath.ToSlash(dir)))
	text := toolText(t, msgs, "1")
	var env struct {
		Files       []any `json:"files"`
		TotalTokens int   `json:"total_tokens"`
		Skipped     int   `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("pack_repo json is not parseable: %v\n%s", err, text)
	}
	if len(env.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(env.Files))
	}
	if env.TotalTokens >= 100 {
		t.Errorf("max_size 10 must omit the oversized body; total_tokens = %d", env.TotalTokens)
	}
	if env.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", env.Skipped)
	}
}

// TestToolCallPackRepoEmptyDirJSON pins that packing an empty directory in
// json format yields a parseable envelope (files null, zero tokens), not an
// error — the MCP mirror of the CLI's TestPackEmptyDirYieldsEmptyBundle.
func TestToolCallPackRepoEmptyDirJSON(t *testing.T) {
	dir := t.TempDir()

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"pack_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","format":"json"}}}`,
	)
	text := toolText(t, msgs, "2")
	var env struct {
		Files       *[]any `json:"files"`
		TotalTokens int    `json:"total_tokens"`
	}
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("empty-dir pack_repo json is not a parseable envelope: %v\n%s", err, text)
	}
	got := 0
	if env.Files != nil {
		got = len(*env.Files)
	}
	if got != 0 || env.TotalTokens != 0 {
		t.Errorf("empty dir pack_repo = %d files, %d tokens; want 0, 0", got, env.TotalTokens)
	}
}

func TestToolCallPackRepoModelAnnotates(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"pack_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","model":"gpt-4o"}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "fit:") {
		t.Errorf("expected fit annotation with model:\n%s", text)
	}
	if !strings.Contains(text, "gpt-4o") {
		t.Errorf("expected model name in annotation:\n%s", text)
	}
}

func TestToolCallPackRepoUnknownModel(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"pack_repo","arguments":{"path":"`+filepath.ToSlash(dir)+`","model":"not-a-real-model"}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "unknown model") {
		t.Errorf("expected unknown model annotation:\n%s", text)
	}
	if !strings.Contains(text, "not-a-real-model") {
		t.Errorf("expected the unknown model name in annotation:\n%s", text)
	}
}

func TestToolCallCountTokensModelFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":"`+filepath.ToSlash(dir)+`","model":"gpt-4o"}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "gpt-4o") {
		t.Errorf("expected only gpt-4o in output:\n%s", text)
	}
	if strings.Contains(text, "claude") {
		t.Errorf("should not contain claude when filtered to gpt-4o:\n%s", text)
	}
}

// TestToolCallCountTokensMaxSize pins count_tokens' max_size argument: a file
// over the cap is estimated from its byte size, so the total is far below the
// full-read count (the mcp mirror of TestTokensMaxSizeUsesEstimate).
func TestToolCallCountTokensMaxSize(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("a", 500)), 0o644)

	full := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q,"format":"json"}}}`,
		filepath.ToSlash(dir)))
	capped := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q,"format":"json","max_size":10}}}`,
		filepath.ToSlash(dir)))
	parse := func(msgs []map[string]any) int {
		var env struct {
			TotalTokens int `json:"total_tokens"`
		}
		if err := json.Unmarshal([]byte(toolText(t, msgs, "1")), &env); err != nil {
			t.Fatalf("count_tokens json is not parseable: %v", err)
		}
		return env.TotalTokens
	}
	if got, want := parse(capped), parse(full); got >= want {
		t.Errorf("max_size 10 must cut the total below the full-read %d, got %d", want, got)
	}
}

// TestToolCallCountTokensMaxDepth pins count_tokens' max_depth argument: a
// two-level file is not counted at max_depth 1, so the total drops below the
// full-tree count (the mcp mirror of TestTokensDepthLimitsTotal).
func TestToolCallCountTokensMaxDepth(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "deep", "x.txt"), []byte(strings.Repeat("b", 200)), 0o644)
	os.WriteFile(filepath.Join(dir, "root.go"), []byte("package main\n"), 0o644)

	full := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q,"format":"json"}}}`,
		filepath.ToSlash(dir)))
	shallow := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q,"format":"json","max_depth":1}}}`,
		filepath.ToSlash(dir)))
	parse := func(msgs []map[string]any) int {
		var env struct {
			TotalTokens int `json:"total_tokens"`
		}
		if err := json.Unmarshal([]byte(toolText(t, msgs, "1")), &env); err != nil {
			t.Fatalf("count_tokens json is not parseable: %v", err)
		}
		return env.TotalTokens
	}
	if got, want := parse(shallow), parse(full); got >= want {
		t.Errorf("max_depth 1 must cut the total below %d, got %d", want, got)
	}
}

// TestToolCallCountTokensNoGitignoreHidden pins count_tokens' no_gitignore and
// hidden arguments: a gitignored file (and a dotfile) raise the total when
// asked for (the mcp mirror of the CLI's gitignore/hidden tests).
func TestToolCallCountTokensNoGitignoreHidden(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("secret.txt\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte(strings.Repeat("s", 200)), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden.go"), []byte("package h\n"), 0o644)

	dflt := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q,"format":"json"}}}`,
		filepath.ToSlash(dir)))
	noGit := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q,"format":"json","no_gitignore":true}}}`,
		filepath.ToSlash(dir)))
	hidden := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q,"format":"json","hidden":true}}}`,
		filepath.ToSlash(dir)))
	parse := func(msgs []map[string]any) int {
		var env struct {
			TotalTokens int `json:"total_tokens"`
		}
		if err := json.Unmarshal([]byte(toolText(t, msgs, "1")), &env); err != nil {
			t.Fatalf("count_tokens json is not parseable: %v", err)
		}
		return env.TotalTokens
	}
	if got, want := parse(noGit), parse(dflt); got <= want {
		t.Errorf("no_gitignore:true must raise the total above %d, got %d", want, got)
	}
	if got, want := parse(hidden), parse(dflt); got <= want {
		t.Errorf("hidden:true must raise the total above %d, got %d", want, got)
	}
}

// TestToolCallCountTokensInclude pins count_tokens' include argument: only
// matching files are counted, so the total drops below the full-tree count.
func TestToolCallCountTokensInclude(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte(strings.Repeat("b", 200)), 0o644)

	full := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q,"format":"json"}}}`,
		filepath.ToSlash(dir)))
	filtered := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q,"format":"json","include":["*.go"]}}}`,
		filepath.ToSlash(dir)))
	parse := func(msgs []map[string]any) int {
		var env struct {
			TotalTokens int `json:"total_tokens"`
		}
		if err := json.Unmarshal([]byte(toolText(t, msgs, "1")), &env); err != nil {
			t.Fatalf("count_tokens json is not parseable: %v", err)
		}
		return env.TotalTokens
	}
	if got, want := parse(filtered), parse(full); got >= want {
		t.Errorf("include *.go must cut the total below %d, got %d", want, got)
	}
}

// TestToolCallCountTokensExclude pins count_tokens' exclude argument: a
// dropped file leaves the counted tree, shrinking the total (the mirror of
// TestToolCallCountTokensInclude).
func TestToolCallCountTokensExclude(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "drop.txt"), []byte(strings.Repeat("b", 200)), 0o644)

	full := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q,"format":"json"}}}`,
		filepath.ToSlash(dir)))
	filtered := serveLines(t, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q,"format":"json","exclude":["*.txt"]}}}`,
		filepath.ToSlash(dir)))
	parse := func(msgs []map[string]any) int {
		var env struct {
			TotalTokens int `json:"total_tokens"`
		}
		if err := json.Unmarshal([]byte(toolText(t, msgs, "1")), &env); err != nil {
			t.Fatalf("count_tokens json is not parseable: %v", err)
		}
		return env.TotalTokens
	}
	if got, want := parse(filtered), parse(full); got >= want {
		t.Errorf("exclude *.txt must cut the total below %d, got %d", want, got)
	}
}

func TestToolCallCountTokensTop(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":"`+filepath.ToSlash(dir)+`","top":3}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	// Should have exactly 3 model lines after the header.
	lines := strings.Split(text, "\n")
	modelLines := 0
	for _, line := range lines {
		if strings.Contains(line, " — ") {
			modelLines++
		}
	}
	if modelLines != 3 {
		t.Errorf("expected 3 models with top=3, got %d:\n%s", modelLines, text)
	}
	// top alone means "the N largest context windows", same as the CLI — the
	// first row must be a 2M-window Gemini model, not the first rows of the
	// registration order (gpt-3.5-turbo & co, which a name sort would produce).
	firstModel := ""
	for _, line := range lines {
		if strings.Contains(line, " — ") {
			parts := strings.Split(line, " — ")
			if len(parts) > 0 {
				firstModel = strings.TrimSpace(parts[0])
			}
			break
		}
	}
	if !strings.Contains(firstModel, "gemini") {
		t.Errorf("expected gemini first with top=3, got %q:\n%s", firstModel, text)
	}
}

func TestToolCallCountTokensSortByWindow(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":"`+filepath.ToSlash(dir)+`","sort":"window","top":3}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	// First model should be gemini-1.5-pro or gemini-2.5-pro (largest window = 2M).
	lines := strings.Split(text, "\n")
	firstModel := ""
	for _, line := range lines {
		if strings.Contains(line, " — ") {
			parts := strings.Split(line, " — ")
			if len(parts) > 0 {
				firstModel = strings.TrimSpace(parts[0])
			}
			break
		}
	}
	if !strings.Contains(firstModel, "gemini") {
		t.Errorf("expected gemini first when sorting by window, got %q:\n%s", firstModel, text)
	}
}

func TestToolCallCountTokensSortJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644)

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":"`+filepath.ToSlash(dir)+`","format":"json","sort":"window","top":2}}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, text)
	}
	fits, _ := env["fits"].([]any)
	if len(fits) != 2 {
		t.Errorf("expected 2 fits with top=2, got %d", len(fits))
	}
	// The fit field is named "model", matching the CLI's tokens --json.
	first, _ := fits[0].(map[string]any)
	if first["model"] == "" {
		t.Errorf("fit is missing the model field: %v", first)
	}
	// window = limit + reserve must hold, like the CLI envelope.
	win, _ := first["window"].(float64)
	lim, _ := first["limit"].(float64)
	res, _ := env["reserve_tokens"].(float64)
	if win != lim+res {
		t.Errorf("window (=%v) must equal limit (=%v) + reserve_tokens (=%v)", win, lim, res)
	}
}

func TestHumanTokens(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0k"},
		{12345, "12.3k"},
		{999999, "1000.0k"},
		{1_000_000, "1.0M"},
		{2_500_000, "2.5M"},
	}
	for _, c := range cases {
		if got := humanTokens(c.in); got != c.want {
			t.Errorf("humanTokens(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- doctor ---

// An agent that gets "git error: exit status 128" out of diff_repo has no way
// to tell whether git is missing, on the wrong version, or simply the ref that
// was wrong. doctor is the call that answers it without a path — and a path is
// exactly the thing the agent may be unable to name.
func TestDoctorToolText(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"doctor","arguments":{}}}`)
	msg := respByID(msgs, 1)
	if msg == nil {
		t.Fatal("no response")
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result object: %v", msg)
	}
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("doctor reported an error: %v", res)
	}
	text := toolText(t, msgs, "1")
	for _, want := range []string{
		"ctxpack diagnostics:",
		"  Version:   ",
		"  Go:        ",
		"  Platform:  ",
		"  Git:       ",
		"  Vendor breakdown:",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("doctor text is missing %q:\n%s", want, text)
		}
	}
	if want := fmt.Sprintf("%d models, %d vendors", len(counter.Models()), distinctVendors()); !strings.Contains(text, want) {
		t.Errorf("doctor text does not state the registry size %q:\n%s", want, text)
	}
}

func TestDoctorToolJSON(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"doctor","arguments":{"format":"json"}}}`)
	msg := respByID(msgs, 1)
	if msg == nil {
		t.Fatal("no response")
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result object: %v", msg)
	}
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("doctor reported an error: %v", res)
	}
	text := toolText(t, msgs, "1")
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("doctor JSON is not valid JSON: %v\n%s", err, text)
	}
	for _, key := range []string{"version", "go_version", "platform", "git", "model_count", "model_vendors", "vendors"} {
		if _, ok := data[key]; !ok {
			t.Errorf("doctor JSON is missing key %q: %s", key, text)
		}
	}
	if data["model_count"].(float64) != float64(len(counter.Models())) {
		t.Errorf("model_count = %v, want %d", data["model_count"], len(counter.Models()))
	}
	// The breakdown is always complete: truncation is a text-only concern.
	if vendors, ok := data["vendors"].([]any); !ok || len(vendors) != distinctVendors() {
		t.Errorf("vendors has %d entries, want %d: %s",
			func() int {
				if v, ok := data["vendors"].([]any); ok {
					return len(v)
				}
				return -1
			}(),
			distinctVendors(), text)
	}
}

// TestDoctorToolTopTruncatesTextOnly pins the split: top shrinks the text
// vendor list but must not shrink the JSON, and must not change the totals in
// either. A report whose counts moved when the reader asked for fewer rows
// would understate the registry.
func TestDoctorToolTopTruncatesTextOnly(t *testing.T) {
	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"doctor","arguments":{"top":1}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"doctor","arguments":{"format":"json","top":1}}}`,
	)
	text := toolText(t, msgs, "1")
	if got := strings.Count(text, " model(s)"); got != 1 {
		t.Errorf("text with top=1 has %d vendor lines, want 1:\n%s", got, text)
	}
	if want := fmt.Sprintf("%d models, %d vendors", len(counter.Models()), distinctVendors()); !strings.Contains(text, want) {
		t.Errorf("top=1 changed the totals in text:\n%s", text)
	}
	jsonText := toolText(t, msgs, "2")
	var data map[string]any
	if err := json.Unmarshal([]byte(jsonText), &data); err != nil {
		t.Fatalf("JSON unmarshal: %v\n%s", err, jsonText)
	}
	if vendors, ok := data["vendors"].([]any); !ok || len(vendors) != distinctVendors() {
		t.Errorf("JSON dropped vendors under top=1: got %d, want %d",
			func() int {
				if v, ok := data["vendors"].([]any); ok {
					return len(v)
				}
				return -1
			}(),
			distinctVendors())
	}
}

// doctor must reject a format it cannot render. parseFormat also accepts xml
// and markdown, which doctor does not support, so a format:xml call has to fail
// rather than quietly returning text.
func TestDoctorToolUnknownFormat(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"doctor","arguments":{"format":"xml"}}}`)
	msg := respByID(msgs, 1)
	if msg == nil {
		t.Fatal("no response")
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result object: %v", msg)
	}
	if isErr, _ := res["isError"].(bool); !isErr {
		t.Fatalf("doctor with format:xml did not report an error: %v", res)
	}
	if text := toolText(t, msgs, "1"); !strings.Contains(text, "unknown format") {
		t.Errorf("error text: %q", text)
	}
}

// An MCP client discovers a tool by reading its schema, so doctor must
// advertise both arguments it accepts and must not require either: requiring
// one would defeat the tool's purpose of working with an empty argument object.
func TestDoctorSchemaOffersBothAndRequiresNeither(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	msg := respByID(msgs, 1)
	if msg == nil {
		t.Fatal("no response")
	}
	tools, _ := msg["result"].(map[string]any)["tools"].([]any)
	var schema map[string]any
	for _, tv := range tools {
		tool := tv.(map[string]any)
		if tool["name"] == "doctor" {
			schema = tool["inputSchema"].(map[string]any)
			break
		}
	}
	if schema == nil {
		t.Fatal("doctor absent from tools/list")
	}
	props, _ := schema["properties"].(map[string]any)
	if len(props) != 2 {
		t.Errorf("doctor schema has %d properties, want 2: %v", len(props), props)
	}
	for _, name := range []string{"format", "top"} {
		if _, ok := props[name].(map[string]any); !ok {
			t.Errorf("doctor schema missing the %s property", name)
		}
	}
	if req, ok := schema["required"].([]any); ok && len(req) != 0 {
		t.Errorf("doctor schema requires %v, want nothing", req)
	}
}

// distinctVendors counts the vendors in the registry, the value doctor's
// model_vendors field and JSON vendors array must both report.
func distinctVendors() int {
	seen := map[string]bool{}
	for _, m := range counter.Models() {
		seen[m.Vendor] = true
	}
	return len(seen)
}

// --- version ---

func TestVersionToolText(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"version","arguments":{}}}`)
	text := toolText(t, msgs, "1")
	if !strings.Contains(text, "ctxpack ") || !strings.Contains(text, "commit ") {
		t.Errorf("version text = %q, want the build identity", text)
	}
}

func TestVersionToolJSON(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"version","arguments":{"format":"json"}}}`)
	text := toolText(t, msgs, "1")
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("version json is not valid JSON: %v\n%s", err, text)
	}
	for _, field := range []string{"name", "version", "os", "arch", "go", "commit", "built"} {
		if _, ok := v[field]; !ok {
			t.Errorf("version json missing %q: %v", field, v)
		}
	}
	if v["name"] != "ctxpack" {
		t.Errorf("version json name = %v, want ctxpack", v["name"])
	}
}

func TestVersionToolUnknownFormat(t *testing.T) {
	msgs := serveLines(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"version","arguments":{"format":"xml"}}}`)
	msg := respByID(msgs, 1)
	if msg == nil {
		t.Fatal("no response")
	}
	res, _ := msg["result"].(map[string]any)
	if isErr, _ := res["isError"].(bool); !isErr {
		t.Errorf("version with format:xml must set isError: %v", msg)
	}
}

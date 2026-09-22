package mcp

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

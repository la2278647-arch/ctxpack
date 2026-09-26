package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// packJSON returns the decoded envelope of one pack_repo format:json call.
// The whole point of asserting on a decoded value rather than a substring is
// that the text must actually be parseable: grepping for a key passes while
// JSON.parse throws.
func packJSON(t *testing.T, args string) map[string]any {
	t.Helper()
	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"pack_repo","arguments":`+args+`}}`,
	)
	msg := respByID(msgs, 2)
	if msg == nil {
		t.Fatalf("no tools/call response: %v", msgs)
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", msg)
	}
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	text := toolText(t, msgs, "2")
	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("tool text is not valid JSON: %v\n%s", err, text)
	}
	return env
}

// TestPackRepoJSONWithModelStaysParseable is the regression for format:"json"
// plus model: the HTML comment the text formats carry was being prepended to
// the JSON document, so a client that JSON.parse's the tool result threw on
// every annotated call. The annotation must travel inside the envelope.
func TestPackRepoJSONWithModelStaysParseable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := packJSON(t, `{"path":`+jsonQuote(dir)+`,"format":"json","model":"gpt-4o"}`)

	if _, ok := env["fit"]; !ok {
		t.Fatalf("fit annotation missing from the JSON envelope: %v", env)
	}
	fit, ok := env["fit"].(map[string]any)
	if !ok {
		t.Fatalf("fit is not an object: %v", env["fit"])
	}
	if name, _ := fit["name"].(string); name != "gpt-4o" {
		t.Errorf("fit.name = %v, want gpt-4o", fit["name"])
	}
	if fits, _ := fit["fits"].(bool); !fits {
		t.Errorf("a one-file repository should fit gpt-4o: %v", fit)
	}
	if pct, _ := fit["pct_used"].(float64); pct < 0 || pct > 100 {
		t.Errorf("fit.pct_used = %v, want 0-100 for a fitting pack", fit["pct_used"])
	}
	// The bundle's own keys must survive the round trip.
	if env["root"] == nil || env["files"] == nil || env["total_tokens"] == nil {
		t.Errorf("the bundle envelope lost its own keys: %v", env)
	}
}

// A comment anywhere in the document makes it unparsable, so assert on the
// decoded value rather than trusting the renderer.
func TestPackRepoJSONNeverCarriesAComment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tool := range []string{"pack_repo"} {
		msgs := serveLines(t,
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"`+tool+`","arguments":{"path":`+
				jsonQuote(dir)+`,"format":"json","model":"gpt-4o"}}}`,
		)
		text := toolText(t, msgs, "2")
		if strings.Contains(text, "<!--") {
			t.Errorf("%s format:json still carries an HTML comment:\n%s", tool, text)
		}
		if !json.Valid([]byte(text)) {
			t.Errorf("%s format:json is not valid JSON:\n%s", tool, text)
		}
	}
}

// Text formats keep the comment: it is a legitimate decoration there and the
// fit information must not be lost just because JSON forbids it.
func TestPackRepoTextWithModelStillCarriesTheComment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, f := range []string{"xml", "markdown", "text"} {
		msgs := serveLines(t,
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"pack_repo","arguments":{"path":`+
				jsonQuote(dir)+`,"format":"`+f+`","model":"gpt-4o"}}}`,
		)
		text := toolText(t, msgs, "2")
		if !strings.Contains(text, "<!-- fit: ") {
			t.Errorf("format:%s lost the fit comment:\n%s", f, text)
		}
	}
}

// A repository that overflows the smallest window must say so in JSON too:
// an agent that cannot read the fit would happily pack past a model's limit
// and the JSON path would be the one that stayed silent.
func TestPackRepoJSONModelReportsOverflow(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 60; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("doc%02d.txt", i)),
			[]byte(strings.Repeat("the quick brown fox jumps over the lazy dog. ", 200)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	env := packJSON(t, `{"path":`+jsonQuote(dir)+`,"format":"json","model":"gpt-4"}`)
	fit, ok := env["fit"].(map[string]any)
	if !ok {
		t.Fatalf("no fit object: %v", env["fit"])
	}
	if fits, _ := fit["fits"].(bool); fits {
		t.Errorf("a repository this large must not report fits for gpt-4: %v", fit)
	}
	if pct, _ := fit["pct_used"].(float64); pct <= 100 {
		t.Errorf("fit.pct_used = %v, want >100 for an overflow", fit["pct_used"])
	}
	if limit, _ := fit["limit"].(float64); limit != float64(8192-4096) {
		t.Errorf("fit.limit = %v, want %d (window minus the reply reserve)", fit["limit"], 8192-4096)
	}
	// The same repository still fits the largest window (gemini-1.5-pro, 2M),
	// so the overflow is about gpt-4's limit, not about the repository being
	// broken.
	env2 := packJSON(t, `{"path":`+jsonQuote(dir)+`,"format":"json","model":"gemini-1.5-pro"}`)
	fit2, ok := env2["fit"].(map[string]any)
	if !ok {
		t.Fatalf("no fit object: %v", env2["fit"])
	}
	if fits, _ := fit2["fits"].(bool); !fits {
		t.Errorf("the same repository should fit the 2M window: %v", fit2)
	}
}

// An unknown model must not corrupt the envelope: the JSON stays parseable and
// says plainly that the model is not in the registry.
func TestPackRepoJSONUnknownModelKeepsTheEnvelope(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := packJSON(t, `{"path":`+jsonQuote(dir)+`,"format":"json","model":"not-a-real-model"}`)
	if env["root"] == nil || env["files"] == nil {
		t.Errorf("an unknown model must not destroy the envelope: %v", env)
	}
	fit, ok := env["fit"].(map[string]any)
	if !ok {
		t.Fatalf("no fit object: %v", env["fit"])
	}
	if unknown, _ := fit["unknown"].(bool); !unknown {
		t.Errorf("the unknown model must be flagged: %v", fit)
	}
	if fits, present := fit["fits"]; present {
		t.Errorf("an unknown model must not claim a fit verdict: %v", fits)
	}
}

// fitNote is the single source of truth for both renderings, so test it once
// directly: the text note and the JSON object must agree on the verdict.
func TestFitNoteTextAndJSONAgree(t *testing.T) {
	for _, tc := range []struct {
		tokens  int
		model   string
		wantFit bool
	}{
		{100, "gpt-4", true},
		{8000, "gpt-4", false}, // gpt-4's limit is 8192-4096 = 4096
		{200000, "gemini-1.5-pro", true},
	} {
		data, note := fitNote(tc.tokens, tc.model)
		if data == nil {
			t.Fatalf("fitNote returned no data for %v", tc)
		}
		fits, ok := data["fits"].(bool)
		if !ok {
			t.Fatalf("fitNote %v: fits is not a bool: %v", tc, data)
		}
		if fits != tc.wantFit {
			t.Errorf("fitNote(%d, %s).fits = %v, want %v", tc.tokens, tc.model, fits, tc.wantFit)
		}
		wantMark := "FITS"
		if !tc.wantFit {
			wantMark = "OVERFLOW"
		}
		if !strings.Contains(note, wantMark) {
			t.Errorf("fitNote(%d, %s) note says %q, want %s", tc.tokens, tc.model, note, wantMark)
		}
	}
}

func TestAnnotateFitJSONIsIdempotentInShape(t *testing.T) {
	// A second pass over an already-annotated envelope must still parse and
	// still carry exactly one fit object.
	in := `{"root":"r","files":[],"total_tokens":5,"total_bytes":10}`
	out := annotateFitJSON(in, 5, "gpt-4o")
	var first map[string]any
	if err := json.Unmarshal([]byte(out), &first); err != nil {
		t.Fatalf("first pass is not valid JSON: %v\n%s", err, out)
	}
	out2 := annotateFitJSON(out, 5, "gpt-4o")
	var second map[string]any
	if err := json.Unmarshal([]byte(out2), &second); err != nil {
		t.Fatalf("second pass is not valid JSON: %v\n%s", err, out2)
	}
	if _, ok := second["fit"].(map[string]any); !ok {
		t.Errorf("the re-annotated envelope lost its fit object: %v", second["fit"])
	}
	if len(second) != len(first) {
		t.Errorf("re-annotation changed the envelope shape: %d keys vs %d", len(second), len(first))
	}
}

// diff_repo shares callPackRepo's annotation path, so it needs the same
// protection: a diff bundle with a model must stay parseable JSON.
func TestDiffRepoJSONWithModelStaysParseable(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":`+
			jsonQuote(dir)+`,"format":"json","model":"gpt-4o"}}}`,
	)
	result := resultByID(msgs, 2)
	if result == nil {
		t.Fatalf("no result: %v", msgs)
	}
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("unexpected error: %v", result["content"])
	}
	text := toolText(t, msgs, "2")
	if !json.Valid([]byte(text)) {
		t.Fatalf("diff_repo format:json with a model is not valid JSON:\n%s", text)
	}
	var env map[string]any
	json.Unmarshal([]byte(text), &env)
	fit, ok := env["fit"].(map[string]any)
	if !ok {
		t.Fatalf("fit missing from the diff envelope: %v", env["fit"])
	}
	if name, _ := fit["name"].(string); name != "gpt-4o" {
		t.Errorf("fit.name = %v, want gpt-4o", fit["name"])
	}
}

// A ref git cannot resolve is the diff_repo error that had no test: it reaches
// the "git error:" branch rather than the not-a-repository one, and it must
// still come back as result.isError rather than a JSON-RPC error object.
func TestDiffRepoUnresolvableRefReportsAGitError(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")

	msgs := serveLines(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"diff_repo","arguments":{"path":`+
			jsonQuote(dir)+`,"ref":"does-not-exist"}}}`,
	)
	// The error rides inside result.isError; the envelope must not be a
	// JSON-RPC error object, or a client that only checks isError misses it.
	result := resultByID(msgs, 2)
	if result == nil {
		t.Fatalf("an unresolvable ref must still answer inside the envelope: %v", msgs)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Fatalf("an unresolvable ref must set result.isError: %v", result)
	}
	text := toolText(t, msgs, "2")
	if !strings.Contains(text, "git error:") {
		t.Errorf("the error should name the git failure: %q", text)
	}
}

// jsonQuote JSON-quotes a path so a test template can embed it in an
// arguments object. filepath.ToSlash is not enough: backslashes must be
// escaped for the JSON layer as well.
func jsonQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// resultByID is respByID's alias for the tools/call result object, since these
// tests always want the result map rather than the whole message.
func resultByID(msgs []map[string]any, id int) map[string]any {
	m := respByID(msgs, id)
	if m == nil {
		return nil
	}
	return m["result"].(map[string]any)
}

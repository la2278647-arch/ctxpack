package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The tool names below are the ones promised in the package doc at the top of
// server.go. Keeping them in a test is what makes that sentence checkable: the
// doc says "exposes six tools" and lists them, so this fails if the list in the
// doc and the list in tools() stop matching.
var documentedToolNames = []string{
	"pack_repo", "repo_map", "count_tokens", "list_models", "diff_repo", "doctor",
}

func TestToolSchemasDocumentEveryProperty(t *testing.T) {
	tools := tools()
	got := make([]string, len(tools))
	for i, tool := range tools {
		got[i] = tool["name"].(string)
	}
	sort.Strings(got)
	want := append([]string(nil), documentedToolNames...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tools() advertises %v, the package doc promises %v", got, want)
	}

	for _, tool := range tools {
		name := tool["name"].(string)
		toolDesc, _ := tool["description"].(string)
		if strings.TrimSpace(toolDesc) == "" {
			t.Errorf("%s: the tool description is empty", name)
		}
		schema := tool["inputSchema"].(map[string]any)
		if schema["type"] != "object" {
			t.Errorf("%s: inputSchema.type = %v, want object", name, schema["type"])
		}
		props, _ := schema["properties"].(map[string]any)
		if props == nil {
			t.Errorf("%s: inputSchema has no properties map", name)
			continue
		}
		for _, pname := range sortedKeys(props) {
			spec := props[pname].(map[string]any)
			if typ, ok := spec["type"]; !ok || typ == "" {
				t.Errorf("%s.%s: no type declared, a client cannot validate it", name, pname)
			}
			desc, _ := spec["description"].(string)
			if strings.TrimSpace(desc) == "" {
				t.Errorf("%s.%s: no description, a client shows an empty hint", name, pname)
			}
		}
		for _, r := range toStrSlice(schema["required"]) {
			if _, ok := props[r]; !ok {
				t.Errorf("%s: required %q is not in properties", name, r)
			}
		}
	}
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Every advertised property is a promise to the caller: tools/call must accept
// it. The value is derived from the schema itself so the test tracks the schema
// instead of hard-coding a second copy of it. A property a handler silently
// ignores still passes here — catching that needs a behavioural test, which
// TestToolCallDiffRepoListHonoursInclude provides for the one that mattered.
func TestEveryAdvertisedPropertyIsAccepted(t *testing.T) {
	dir := newAcceptRepo(t)

	for _, tool := range tools() {
		name := tool["name"].(string)
		props := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
		args := map[string]any{}
		for pname, specAny := range props {
			spec := specAny.(map[string]any)
			args[pname] = schemaValue(t, name, pname, spec, dir)
		}
		msgs := serveLines(t,
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"`+name+`","arguments":`+
				mustJSON(args)+`}}`,
		)
		msg := respByID(msgs, 2)
		if msg == nil {
			t.Fatalf("%s: no response", name)
		}
		if e, ok := msg["error"].(map[string]any); ok {
			t.Errorf("%s: tools/call returned an RPC error %v", name, e["message"])
			continue
		}
		result, _ := msg["result"].(map[string]any)
		if result == nil {
			t.Fatalf("%s: no result", name)
		}
		if result["isError"] == true {
			content, _ := result["content"].([]any)
			text := ""
			if len(content) > 0 {
				if m, ok := content[0].(map[string]any); ok {
					text, _ = m["text"].(string)
				}
			}
			t.Errorf("%s rejected its own advertised arguments %v:\n  %s", name, args, text)
		}
	}
}

// schemaValue picks one value that satisfies the declared schema, so a change
// to tools() is picked up here rather than silently drifting from it.
func schemaValue(t *testing.T, tool, prop string, spec map[string]any, dir string) any {
	t.Helper()
	if enum := firstEnum(spec); enum != "" {
		return enum
	}
	switch typ := spec["type"].(string); typ {
	case "string":
		switch prop {
		case "path":
			return filepath.ToSlash(dir)
		case "ref":
			return "WORKTREE"
		case "model":
			return "gpt-4o"
		}
		t.Fatalf("%s.%s: no value known for a free string", tool, prop)
	case "boolean":
		return true
	case "integer":
		// 1 is valid for budget, top, max_size and max_depth; each of those
		// degrades to a smaller result instead of an error.
		return 1
	case "array":
		return []any{"*"}
	default:
		t.Fatalf("%s.%s: unexpected type %q", tool, prop, typ)
	}
	return nil
}

// firstEnum returns the first declared enum value, accepting the []string the
// schemas are written with as well as the []any json.Unmarshal produces.
func firstEnum(spec map[string]any) string {
	switch e := spec["enum"].(type) {
	case []string:
		if len(e) > 0 {
			return e[0]
		}
	case []any:
		if len(e) > 0 {
			if s, ok := e[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// newAcceptRepo builds a repository with committed files and one uncommitted
// edit, so every tool that needs a repository or a diff has something to work
// on instead of failing on an empty state.
func newAcceptRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for p, body := range map[string]string{
		"README.md": "hello\n",
		"src/a.go":  "package a\n",
		"notes.txt": "notes\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitInit(t, dir)
	gitAddAll(t, dir)
	gitCommit(t, dir, "initial")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes\n\nmore\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

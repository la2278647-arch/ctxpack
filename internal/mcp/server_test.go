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

// TestServeRoundTrip drives the protocol through a single Serve call, covering
// initialize, tools/list, ping, an unknown method, and a notification (which
// must not produce a response).
func TestServeRoundTrip(t *testing.T) {
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping","params":{}}`,
		`{"jsonrpc":"2.0","id":4,"method":"does/not/exist","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := Serve(strings.NewReader(in), &out, "test"); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var byID = map[string]map[string]any{}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("response is not valid JSON: %v: %s", err, line)
		}
		if m["jsonrpc"] != "2.0" {
			t.Errorf("jsonrpc field: %v", m["jsonrpc"])
		}
		byID[fmt.Sprint(m["id"])] = m
		n++
	}

	// Five requests, but the notification must not be answered.
	if n != 4 {
		t.Fatalf("responses: got %d, want 4 (notification excluded)", n)
	}

	if _, ok := byID["1"]["result"]; !ok {
		t.Error("initialize: missing result")
	}
	tools, ok := byID["2"]["result"].(map[string]any)["tools"].([]any)
	if !ok || len(tools) != 4 {
		t.Fatalf("tools/list: %v", byID["2"])
	}
	if _, ok := byID["3"]["result"]; !ok {
		t.Error("ping: missing result")
	}
	if _, ok := byID["4"]["error"]; !ok {
		t.Error("unknown method: must return an error")
	}
}

func TestCountTokensTool(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":%q}}}`, dir),
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"count_tokens","arguments":{}}}`,
	}, "\n") + "\n"

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
			t.Fatal(err)
		}
		msgs = append(msgs, m)
	}

	// Success path: text payload mentions the token count.
	text := toolText(t, msgs, "2")
	if !strings.Contains(text, "Tokens:") {
		t.Errorf("count_tokens text: %q", text)
	}

	// Missing path: isError must be set inside the tool result, not a crash.
	last := msgs[len(msgs)-1]
	res, ok := last["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing path: no result object: %v", last)
	}
	if isErr, _ := res["isError"].(bool); !isErr {
		t.Errorf("missing path must set result.isError: %v", last)
	}
	if text := toolText(t, msgs, "3"); !strings.Contains(text, "missing required argument") {
		t.Errorf("error text: %q", text)
	}
}

// toolText pulls the first text content item out of a tools/call response.
func toolText(t *testing.T, msgs []map[string]any, id string) string {
	t.Helper()
	for _, m := range msgs {
		if fmt.Sprint(m["id"]) != id {
			continue
		}
		res, ok := m["result"].(map[string]any)
		if !ok {
			return ""
		}
		items, ok := res["content"].([]any)
		if !ok || len(items) == 0 {
			return ""
		}
		first, ok := items[0].(map[string]any)
		if !ok {
			return ""
		}
		return fmt.Sprint(first["text"])
	}
	return ""
}

func TestParseError(t *testing.T) {
	var out bytes.Buffer
	if err := Serve(strings.NewReader("this is not json\n"), &out, "test"); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out.String()), &m); err != nil {
		t.Fatalf("parse error response invalid: %v: %s", err, out.String())
	}
	errObj, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected an error response, got %v", m)
	}
	if code, _ := errObj["code"].(float64); code != -32700 {
		t.Errorf("parse error code: %v, want -32700", errObj["code"])
	}
}

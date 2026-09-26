package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadmeMCPTableMatchesTheToolSchemas keeps the README's MCP tool table
// honest against tools(). This is the same class of drift as the help text:
// when `doctor` was added in v0.1.8 the table was never updated, so a client
// reading the README concluded that a failing pack could not be diagnosed from
// inside the server — the very case doctor exists for.
func TestReadmeMCPTableMatchesTheToolSchemas(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("README.md: %v", err)
	}

	got, ok := parseToolTable(string(readme))
	if !ok {
		t.Fatalf("README has no parseable MCP tool table\nlooking for a row: | Tool | Arguments |")
	}

	want := toolSchemas()
	if len(got) != len(want) {
		t.Errorf("the table lists %d tools, the server advertises %d", len(got), len(want))
	}

	for name, ws := range want {
		gs, present := got[name]
		if !present {
			t.Errorf("the table does not document %s; the server advertises it", name)
			continue
		}
		if len(gs) != len(ws) {
			t.Errorf("%s: the table lists %v, the schema declares %v", name, keysOf(gs), keysOf(ws))
			continue
		}
		for arg, optional := range ws {
			gotOptional, known := gs[arg]
			if !known {
				t.Errorf("%s: the schema declares %s but the table omits it", name, arg)
				continue
			}
			if gotOptional != optional {
				verb := "optional"
				if optional {
					verb = "required"
				}
				t.Errorf("%s: %s is documented as %s, the schema says %s", name, arg, verb, verbFor(optional))
			}
		}
	}
	for name := range got {
		if _, known := want[name]; !known {
			t.Errorf("the table documents %s, which the server does not advertise", name)
		}
	}
}

// toolSchemas is the ground truth the README has to match: for each tool, every
// declared property and whether it is required.
func toolSchemas() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, tool := range tools() {
		name, _ := tool["name"].(string)
		schema, _ := tool["inputSchema"].(map[string]any)
		if name == "" {
			continue
		}
		props, _ := schema["properties"].(map[string]any)
		required := map[string]bool{}
		if req, ok := schema["required"].([]string); ok {
			for _, r := range req {
				required[r] = true
			}
		}
		declared := map[string]bool{}
		for arg := range props {
			// false = required by the schema, so a bare name in the README.
			declared[arg] = !required[arg]
		}
		out[name] = declared
	}
	return out
}

// parseToolTable reads the "| Tool | Arguments |" table. Arguments are written
// as backticked names, an optional one with a trailing "?", so the parse does
// not have to guess where an argument list ends and prose begins.
func parseToolTable(readme string) (map[string]map[string]bool, bool) {
	lines := strings.Split(readme, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "| Tool | Arguments |" {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, false
	}

	out := map[string]map[string]bool{}
	for _, line := range lines[start+2:] { // skip the header and its rule
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			break
		}
		cells := splitRow(line)
		if len(cells) < 2 {
			continue
		}
		name := backtickCells(cells[0])
		if name == "" {
			continue
		}
		args := map[string]bool{}
		for _, a := range backticked(cells[1]) {
			if strings.HasSuffix(a, "?") {
				args[strings.TrimSuffix(a, "?")] = true
			} else {
				args[a] = false
			}
		}
		out[name] = args
	}
	return out, true
}

// backtickCells returns the single backticked token in a cell, or "" if there
// is not exactly one.
func backtickCells(cell string) string {
	toks := backticked(cell)
	if len(toks) != 1 {
		return ""
	}
	return toks[0]
}

func backticked(cell string) []string {
	var out []string
	for {
		open := strings.Index(cell, "`")
		if open < 0 {
			break
		}
		cell = cell[open+1:]
		close := strings.Index(cell, "`")
		if close < 0 {
			break
		}
		out = append(out, cell[:close])
		cell = cell[close+1:]
	}
	return out
}

func splitRow(row string) []string {
	row = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(row), "|"), "|")
	return strings.Split(row, "|")
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func verbFor(optional bool) string {
	if optional {
		return "optional"
	}
	return "required"
}

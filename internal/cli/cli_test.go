package cli

import (
	"testing"
)

// TestReorderArgs pins the argument reordering that makes
// `ctxpack pack ./repo --format markdown` work. Go's flag package stops parsing
// at the first non-flag token, so without this the flags after the path would
// be silently ignored.
func TestReorderArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
		want []string
	}{
		{"path first", []string{".", "--format", "markdown"}, []string{"--format", "markdown", "."}},
		{"path last", []string{"--format", "markdown", "."}, []string{"--format", "markdown", "."}},
		{"equals form", []string{"--format=markdown", "."}, []string{"--format=markdown", "."}},
		{"no flags", []string{"."}, []string{"."}},
		{"repeatable", []string{".", "--include", "*.go", "--include", "*.md"},
			[]string{"--include", "*.go", "--include", "*.md", "."}},
		{"dash value", []string{"--output", "-", "."}, []string{"--output", "-", "."}},
		{"empty", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := reorderArgs(tc.in)
			if tc.want == nil {
				if len(got) != 0 {
					t.Fatalf("got %v, want empty", got)
				}
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len %d != %d: %v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("index %d: got %q want %q (full: %v)", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"xml", "xml"},
		{"", "xml"},
		{"MD", "markdown"},
		{"markdown", "markdown"},
		{"JSON", "json"},
		{"text", "text"},
		{"plain", "text"},
		{"raw", "text"},
		{"bogus", "xml"},
	} {
		if got := string(parseFormat(tc.in)); got != tc.want {
			t.Errorf("parseFormat(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

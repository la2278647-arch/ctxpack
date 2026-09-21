package format

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
)

func sample() *Bundle {
	return &Bundle{
		Root: "github.com/acme/widget",
		Files: []File{
			{Path: "README.md", Content: "# Widget\n\nHello <world> & \"friends\".\n", Tokens: 42, Bytes: 30},
			{Path: "internal/thing.go", Content: "package thing\n\nvar X = 1\n", Tokens: 7, Bytes: 23},
		},
		TotalTokens: 49,
		TotalBytes:  53,
		Skipped:     3,
	}
}

func TestRenderDispatch(t *testing.T) {
	b := sample()
	for _, tc := range []struct {
		f    Format
		want string
	}{
		{XML, "<repository>"},
		{Markdown, "# Repository:"},
		{JSON, "{"},
		{Text, "Repository:"},
		// Anything unrecognised falls through to the XML branch.
		{Format("yaml"), "<repository>"},
		{Format(""), "<repository>"},
	} {
		got := Render(b, tc.f)
		if !strings.Contains(got, tc.want) {
			t.Errorf("Render(%s): want %q somewhere in output, got:\n%s", tc.f, tc.want, got)
		}
	}
}

func TestRenderXMLIsWellFormed(t *testing.T) {
	// The whole point of wrapping content in CDATA is that arbitrary source
	// text survives. A strict parser must accept the document.
	var node struct {
		XMLName xml.Name `xml:"repository"`
		Meta    struct {
			Root        string `xml:"root"`
			FileCount   int    `xml:"fileCount"`
			TotalTokens int    `xml:"totalTokens"`
			TotalBytes  int    `xml:"totalBytes"`
			Skipped     int    `xml:"skipped"`
		} `xml:"meta"`
	}
	if err := xml.Unmarshal([]byte(Render(sample(), XML)), &node); err != nil {
		t.Fatalf("rendered XML is not well-formed: %v\n%s", err, Render(sample(), XML))
	}
	if node.XMLName.Local != "repository" {
		t.Errorf("root element = %q, want repository", node.XMLName.Local)
	}
	if node.Meta.Root != sample().Root || node.Meta.FileCount != 2 ||
		node.Meta.TotalTokens != 49 || node.Meta.TotalBytes != 53 || node.Meta.Skipped != 3 {
		t.Errorf("meta round-trip = %+v", node.Meta)
	}
}

func TestRenderXMLEscapesRoot(t *testing.T) {
	b := &Bundle{Root: `a < b & c "d" 'e'`}
	out := Render(b, XML)
	for _, want := range []string{`a &lt; b &amp; c &quot;d&quot; &apos;e&apos;`} {
		if !strings.Contains(out, want) {
			t.Errorf("root not escaped: %s", out)
		}
	}
}

func TestRenderXMLEscapesFilePath(t *testing.T) {
	// '&' is legal in a directory name on every platform, and %q does not
	// escape it, so the file element was not valid XML.
	path := "a&b/x < y.go"
	b := &Bundle{
		Root:        "repo",
		Files:       []File{{Path: path, Tokens: 2, Bytes: 7, Content: "hi\n"}},
		TotalTokens: 2,
		TotalBytes:  7,
	}
	out := renderXML(b)
	var node struct {
		Files struct {
			File []struct {
				Path string `xml:"path,attr"`
			} `xml:"file"`
		} `xml:"files"`
	}
	if err := xml.Unmarshal([]byte(out), &node); err != nil {
		t.Fatalf("not well-formed: %v\n%s", err, out)
	}
	if len(node.Files.File) != 1 || node.Files.File[0].Path != path {
		t.Errorf("path round-trip = %q, want %q", node.Files.File[0].Path, path)
	}
}

func TestRenderXMLContentWithCDataTerminator(t *testing.T) {
	// A "]]>" sequence occurs in real source (e.g. a > b[i]]> 0) and used to
	// close the CDATA section early, leaving the rest of the file as markup.
	content := "a := b[i]]> 0\n"
	b := &Bundle{
		Root:        "repo",
		Files:       []File{{Path: "t.go", Tokens: 1, Bytes: len(content), Content: content}},
		TotalTokens: 1,
		TotalBytes:  len(content),
	}
	out := renderXML(b)
	var node struct {
		Files struct {
			File []struct {
				Path    string `xml:"path,attr"`
				Content string `xml:"content"`
			} `xml:"file"`
		} `xml:"files"`
	}
	// The document must parse, and the content must survive intact. The
	// fragment is split across two CDATA sections, so compare through the
	// parser rather than with strings.Contains on the raw text.
	if err := xml.Unmarshal([]byte(out), &node); err != nil {
		t.Fatalf("not well-formed: %v\n%s", err, out)
	}
	if len(node.Files.File) != 1 {
		t.Fatalf("files = %d, want 1", len(node.Files.File))
	}
	got := node.Files.File[0].Content
	if !strings.Contains(got, content) {
		t.Errorf("content round-trip = %q, want it to contain %q", got, content)
	}
	// The renderer wraps the content in newlines, so the parsed text starts
	// with "\n" + content. Assert that rather than trimming, which would eat
	// a newline that belongs to the source itself.
	if !strings.HasPrefix(node.Files.File[0].Content, "\n"+content) {
		t.Errorf("round-trip = %q, want a prefix of %q", node.Files.File[0].Content, "\n"+content)
	}
}

func TestRenderXMLBinaryMarker(t *testing.T) {
	b := &Bundle{
		Root: "repo",
		Files: []File{
			{Path: "logo.png", Binary: true, Tokens: 0, Bytes: 4096},
		},
		TotalBytes: 4096,
	}
	out := Render(b, XML)
	if !strings.Contains(out, `<content binary="true"/>`) {
		t.Fatalf("binary marker missing: %s", out)
	}
	if strings.Contains(out, "CDATA") {
		t.Fatalf("binary file must not be wrapped in CDATA: %s", out)
	}
}

func TestRenderJSONRoundTrip(t *testing.T) {
	got := Render(sample(), JSON)
	var b Bundle
	if err := json.Unmarshal([]byte(got), &b); err != nil {
		t.Fatalf("rendered JSON did not parse: %v\n%s", err, got)
	}
	if b.Root != sample().Root || b.TotalTokens != 49 || b.TotalBytes != 53 || b.Skipped != 3 {
		t.Errorf("scalar fields mismatch: %+v", b)
	}
	if len(b.Files) != 2 || b.Files[0].Path != "README.md" || b.Files[1].Path != "internal/thing.go" {
		t.Errorf("files mismatch: %+v", b.Files)
	}
	// omitempty: content must be present here, absent for an empty one.
	var empty Bundle
	empty.Files = []File{{Path: "a", Tokens: 1, Bytes: 1}}
	if got := Render(&empty, JSON); strings.Contains(got, `"content"`) || strings.Contains(got, `"binary"`) {
		t.Errorf("omitempty not honoured: %s", got)
	}
}

func TestRenderMarkdownStructure(t *testing.T) {
	out := Render(sample(), Markdown)
	for _, want := range []string{
		"# Repository: github.com/acme/widget",
		"- Files: 2  | Tokens: ~49  | Bytes: 53 B  | Skipped: 3",
		"## `README.md` (42 tokens, 30 B)",
		"## `internal/thing.go` (7 tokens, 23 B)",
		"```markdown",
		"```go",
		"Hello <world> & \"friends\".",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown missing %q\n%s", want, out)
		}
	}
}

func TestRenderTextStructure(t *testing.T) {
	out := Render(sample(), Text)
	for _, want := range []string{
		"Repository: github.com/acme/widget",
		"Files: 2 | Tokens: ~49 | Bytes: 53 B",
		"==== README.md (42 tokens) ====",
		"==== internal/thing.go (7 tokens) ====",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text missing %q\n%s", want, out)
		}
	}
}

func TestRenderTextTerminatesEveryFile(t *testing.T) {
	// Content without a trailing newline must still get one, and every file
	// block must end with a blank line.
	b := &Bundle{Root: "r", Files: []File{{Path: "a.go", Content: "package a", Tokens: 1, Bytes: 9}}}
	out := Render(b, Text)
	if !strings.Contains(out, "package a\n\n") {
		t.Errorf("file block not terminated: %q", out)
	}
	// Already-terminated content must not gain a second newline.
	b.Files[0].Content = "package a\n"
	if !strings.Contains(out, "package a\n\n") || strings.Contains(Render(b, Text), "package a\n\n\n") {
		t.Errorf("double newline: %q", Render(b, Text))
	}
}

func TestRenderBinaryAcrossFormats(t *testing.T) {
	b := &Bundle{
		Root:       "repo",
		Files:      []File{{Path: "logo.png", Binary: true, Tokens: 0, Bytes: 4096}},
		TotalBytes: 4096,
	}
	for _, tc := range []struct {
		f    Format
		want string
	}{
		{XML, `<content binary="true"/>`},
		{Markdown, "_(binary file — content omitted)_"},
		{Text, "[binary file — content omitted]"},
	} {
		if got := Render(b, tc.f); !strings.Contains(got, tc.want) {
			t.Errorf("Render(%s) missing %q\n%s", tc.f, tc.want, got)
		}
	}
	if got := Render(b, JSON); !strings.Contains(got, `"binary": true`) {
		t.Errorf("JSON must carry the binary flag: %s", got)
	}
}

func TestHumanBytes(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	} {
		if got := humanBytes(tc.n); got != tc.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestLangHint(t *testing.T) {
	for _, tc := range []struct {
		path string
		want string
	}{
		{"a.go", "go"},
		{"A.GO", "go"},
		{"a.ts", "ts"},
		{"a.tsx", "ts"},
		{"a.js", "js"},
		{"a.jsx", "js"},
		{"a.py", "python"},
		{"a.rs", "rust"},
		{"a.java", "java"},
		{"a.kt", "kotlin"},
		{"a.rb", "ruby"},
		{"a.sh", "bash"},
		{"a.bash", "bash"},
		{"a.yml", "yaml"},
		{"a.yaml", "yaml"},
		{"a.json", "json"},
		{"a.md", "markdown"},
		{"a.sql", "sql"},
		{"a.c", "c"},
		{"a.h", "c"},
		{"a.cpp", "cpp"},
		{"a.hpp", "cpp"},
		{"a.cc", "cpp"},
		{"a.cs", "csharp"},
		{"a.html", "html"},
		{"a.htm", "html"},
		{"a.css", "css"},
		{"Dockerfile", "dockerfile"},
		{"sub/UPPER.DOCKERFILE", "dockerfile"},
		{"Makefile", ""},
		{"a.unknown", ""},
		{"a", ""},
	} {
		if got := langHint(tc.path); got != tc.want {
			t.Errorf("langHint(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

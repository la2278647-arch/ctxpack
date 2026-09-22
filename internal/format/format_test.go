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
	// The document must parse, and the content must survive byte for byte.
	// The "]]>" splits the body across two CDATA sections, so compare through
	// the parser rather than with strings.Contains on the raw text.
	if err := xml.Unmarshal([]byte(out), &node); err != nil {
		t.Fatalf("not well-formed: %v\n%s", err, out)
	}
	if len(node.Files.File) != 1 {
		t.Fatalf("files = %d, want 1", len(node.Files.File))
	}
	if got := node.Files.File[0].Content; got != content {
		t.Errorf("content round-trip = %q, want exactly %q", got, content)
	}
}

func TestRenderXMLContentRoundTripsExactly(t *testing.T) {
	// The parsed <content> text must equal the source bytes for any content.
	// A newline on either side of the CDATA leaked into the round trip, so a
	// read-back of a file gained a leading and trailing blank line; the table
	// covers the shapes that would have caught that.
	cases := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"no trailing newline", "package a"},
		{"one trailing newline", "package a\n"},
		{"two trailing newlines", "package a\n\n"},
		{"leading newline", "\npackage a"},
		{"only whitespace", "   \n\t\n"},
		{"markup characters", "<a & b> `c`</a>\n"},
		{"cdata terminator", "x := y]]> 0\n"},
		{"cdata terminator at start", "]]> leading\n"},
		{"cdata terminator at end", "trailing ]]>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &Bundle{
				Root:        "repo",
				Files:       []File{{Path: "f.txt", Content: tc.content, Tokens: 1, Bytes: len(tc.content)}},
				TotalTokens: 1,
				TotalBytes:  len(tc.content),
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
			if err := xml.Unmarshal([]byte(out), &node); err != nil {
				t.Fatalf("not well-formed for %q: %v\n%s", tc.content, err, out)
			}
			if len(node.Files.File) != 1 {
				t.Fatalf("files = %d, want 1", len(node.Files.File))
			}
			if got := node.Files.File[0].Content; got != tc.content {
				t.Errorf("round-trip = %q, want exactly %q", got, tc.content)
			}
		})
	}
}

func TestRenderXMLContentDoesNotTouchIndentation(t *testing.T) {
	// The fix hugs the CDATA to the element tags; a regression that puts the
	// pretty-printed whitespace back would show up as an exact-string failure.
	b := &Bundle{
		Root:        "repo",
		Files:       []File{{Path: "a.go", Content: "package a\n", Tokens: 1, Bytes: 9}},
		TotalTokens: 1,
		TotalBytes:  9,
	}
	out := renderXML(b)
	if !strings.Contains(out, "<content><![CDATA[package a\n]]></content>") {
		t.Errorf("content element is not hugged to its CDATA:\n%s", out)
	}
}

func TestRenderXMLNormalizesCarriageReturn(t *testing.T) {
	// XML requires a carriage return in character data to be normalized to a
	// line feed, and encoding/xml does it even when the CR comes through a
	// character reference. The XML format therefore cannot round-trip CRLF,
	// whatever the renderer emits; markdown, json and text do preserve it.
	// Pinning the behavior here keeps a later edit from chasing it as a bug.
	b := &Bundle{
		Root:        "repo",
		Files:       []File{{Path: "a.txt", Content: "a\r\nb\r\n", Tokens: 1, Bytes: 6}},
		TotalTokens: 1,
		TotalBytes:  6,
	}
	var node struct {
		Files struct {
			File []struct {
				Content string `xml:"content"`
			} `xml:"file"`
		} `xml:"files"`
	}
	if err := xml.Unmarshal([]byte(renderXML(b)), &node); err != nil {
		t.Fatalf("not well-formed: %v", err)
	}
	if got := node.Files.File[0].Content; got != "a\nb\n" {
		t.Errorf("CR normalization = %q, want %q", got, "a\nb\n")
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

func TestRenderMarkdownFenceIsExact(t *testing.T) {
	// The closing fence must start on its own line, but the file's bytes must
	// not gain a newline to get there.
	terminated := "package a\nfunc f() {}\n"
	b := &Bundle{
		Root:        "repo",
		Files:       []File{{Path: "a.go", Content: terminated, Tokens: 1, Bytes: len(terminated)}},
		TotalTokens: 1,
		TotalBytes:  len(terminated),
	}
	out := Render(b, Markdown)
	if !strings.Contains(out, "```go\n"+terminated+"```\n\n") {
		t.Errorf("terminated content must be exact:\n%s", out)
	}
	// CRLF survives the markdown fence untouched.
	b.Files[0].Content = "a\r\nb\r\n"
	out = Render(b, Markdown)
	if !strings.Contains(out, "```go\na\r\nb\r\n```\n\n") {
		t.Errorf("crlf content must be exact:\n%q", out)
	}
	// An unterminated source needs one newline for the fence, and only one.
	b.Files[0].Content = "package a"
	out = Render(b, Markdown)
	if !strings.Contains(out, "```go\npackage a\n```\n\n") {
		t.Errorf("unterminated content must gain exactly one newline:\n%s", out)
	}
	if strings.Contains(out, "package a\n\n```\n\n") {
		t.Errorf("unterminated content must not gain two newlines:\n%s", out)
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

func omittedSample() *Bundle {
	b := sample()
	b.Omitted = []string{"vendor/heavy.go", "a <b>&c.md"}
	b.OmittedTokens = 900
	return b
}

func TestRenderXMLOmitted(t *testing.T) {
	var node struct {
		XMLName xml.Name `xml:"repository"`
		Omitted struct {
			Count  int      `xml:"count,attr"`
			Tokens int      `xml:"tokens,attr"`
			Paths  []string `xml:"path"`
		} `xml:"omitted"`
	}
	out := Render(omittedSample(), XML)
	if err := xml.Unmarshal([]byte(out), &node); err != nil {
		t.Fatalf("rendered XML is not well-formed: %v\n%s", err, out)
	}
	if node.Omitted.Count != 2 || node.Omitted.Tokens != 900 {
		t.Errorf("omitted attrs = count=%d tokens=%d, want count=2 tokens=900",
			node.Omitted.Count, node.Omitted.Tokens)
	}
	if len(node.Omitted.Paths) != 2 {
		t.Fatalf("omitted paths = %v", node.Omitted.Paths)
	}
	if node.Omitted.Paths[0] != "vendor/heavy.go" {
		t.Errorf("path[0] = %q", node.Omitted.Paths[0])
	}
	// Markup in a path must be escaped, and come back unescaped on parse.
	if node.Omitted.Paths[1] != "a <b>&c.md" {
		t.Errorf("path[1] = %q, want %q", node.Omitted.Paths[1], "a <b>&c.md")
	}
}

func TestRenderXMLOmittedAbsentWhenEmpty(t *testing.T) {
	out := Render(sample(), XML)
	if strings.Contains(out, "<omitted") {
		t.Errorf("an unlimited pack must not emit an omitted element:\n%s", out)
	}
}

func TestRenderMarkdownOmitted(t *testing.T) {
	out := Render(omittedSample(), Markdown)
	want := "## Omitted by budget (2 files, ~900 tokens)\n\n" +
		"- `vendor/heavy.go`\n- `a <b>&c.md`\n"
	if !strings.Contains(out, want) {
		t.Errorf("markdown missing the omitted section:\n%s", out)
	}
	if i := strings.Index(out, want); i < strings.Index(out, "## `README.md`") {
		t.Error("the omitted section must come after the packed files")
	}
}

func TestRenderMarkdownOmittedAbsentWhenEmpty(t *testing.T) {
	if strings.Contains(Render(sample(), Markdown), "Omitted by budget") {
		t.Error("an unlimited pack must not mention omitted files")
	}
}

func TestRenderTextOmitted(t *testing.T) {
	want := "==== omitted by budget (2 files, ~900 tokens) ====\nvendor/heavy.go\n"
	if got := Render(omittedSample(), Text); !strings.Contains(got, want) {
		t.Errorf("text missing the omitted section:\n%s", got)
	}
	if strings.Contains(Render(sample(), Text), "omitted by budget") {
		t.Error("an unlimited pack must not mention omitted files")
	}
}

func TestRenderJSONOmitted(t *testing.T) {
	var got struct {
		Omitted       []string `json:"omitted"`
		OmittedTokens int      `json:"omitted_tokens"`
	}
	if err := json.Unmarshal([]byte(Render(omittedSample(), JSON)), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Omitted) != 2 || got.OmittedTokens != 900 {
		t.Errorf("json omitted = %v tokens=%d", got.Omitted, got.OmittedTokens)
	}
	// omitempty: an unlimited pack must not carry the keys at all.
	if strings.Contains(Render(sample(), JSON), "omitted") {
		t.Error("empty omitted lists must be omitted from the JSON")
	}
}
